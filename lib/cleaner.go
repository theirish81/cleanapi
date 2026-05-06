// Package lib provides functionality to clean and prune OpenAPI 3 specifications.
// It can selectively remove extensions, examples, non-2xx responses, and unreferenced components
// while preserving the core structure and descriptions.
package lib

import (
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// CleanOptions defines the configuration for the cleaning process.
type CleanOptions struct {
	// CleanExamples, if true, removes 'example' and 'examples' fields from all components.
	CleanExamples bool
	// KeepOperationIDs, if non-empty, specifies the only operation IDs to be kept in the spec.
	// Operations not in this list will be removed, and empty paths will be pruned.
	KeepOperationIDs []string
	// KeepTags, if non-empty, specifies that all operations with these tags should be kept.
	KeepTags []string
	// RemoveNon2xxErrors, if true, removes all responses with a status code that does not start with '2'.
	RemoveNon2xxErrors bool
	// RemoveResponseSchemas, if true, removes the 'content' field from all responses,
	// effectively stripping out response bodies.
	RemoveResponseSchemas bool
	// RemoveExtensions, if true, removes all custom 'x-' extensions from the specification.
	RemoveExtensions bool
	// StripMetadata, if true, removes metadata like ExternalDocs, Contact, and License.
	StripMetadata bool
}

// CleanAndPruneOpenAPI is the main entry point for the cleaning process.
// It modifies the provided openapi3.T document in-place based on the CleanOptions.
func CleanAndPruneOpenAPI(doc *openapi3.T, opts CleanOptions) {
	if doc == nil {
		return
	}

	c := newCleaner(doc, opts)

	c.cleanMetadata()
	c.cleanServers(c.doc.Servers)
	c.cleanPaths()
	c.cleanTags()

	// pruneComponents implements a mark-and-sweep style garbage collector to remove
	// top-level components that are no longer reachable from any operation.
	c.pruneComponents()
}

// cleaner holds the internal state for a single cleaning run.
// State encapsulation prevents side effects and allows for easier tracking of visited nodes
// in recursive operations, which is critical for handling circular references.
type cleaner struct {
	doc     *openapi3.T
	opts    CleanOptions
	keepOps map[*openapi3.Operation]bool
	// isFiltering is true if any operation filtering (by ID or Tag) is active.
	isFiltering bool
	// visitedSchemas tracks pointers to openapi3.Schema objects to prevent infinite recursion
	// during the cleaning phase.
	visitedSchemas map[*openapi3.Schema]bool
	// visitedObjects tracks generic pointers during the tracing phase (GC) to prevent
	// infinite recursion when following references.
	visitedObjects map[interface{}]bool
}

// newCleaner initializes a cleaner with the given document and options.
func newCleaner(doc *openapi3.T, opts CleanOptions) *cleaner {
	keepOps, isFiltering := buildKeepOpsMap(doc, opts)

	return &cleaner{
		doc:            doc,
		opts:           opts,
		keepOps:        keepOps,
		isFiltering:    isFiltering,
		visitedSchemas: make(map[*openapi3.Schema]bool),
		visitedObjects: make(map[interface{}]bool),
	}
}

// buildKeepOpsMap identifies which operations should be preserved based on IDs and Tags.
func buildKeepOpsMap(doc *openapi3.T, opts CleanOptions) (map[*openapi3.Operation]bool, bool) {
	keepOps := make(map[*openapi3.Operation]bool)
	isFiltering := len(opts.KeepOperationIDs) > 0 || len(opts.KeepTags) > 0

	if !isFiltering || doc.Paths == nil {
		return keepOps, isFiltering
	}

	ids := make(map[string]bool)
	for _, id := range opts.KeepOperationIDs {
		ids[id] = true
	}
	tags := make(map[string]bool)
	for _, t := range opts.KeepTags {
		tags[t] = true
	}

	for _, pathItem := range doc.Paths.Map() {
		for _, op := range pathItem.Operations() {
			// Selection logic: match tag AND match ID if both are provided.
			// If only one criteria is provided, match that one.

			matchTag := len(tags) == 0
			if !matchTag {
				for _, t := range op.Tags {
					if tags[t] {
						matchTag = true
						break
					}
				}
			}

			matchID := len(ids) == 0
			if !matchID {
				if op.OperationID != "" && ids[op.OperationID] {
					matchID = true
				}
			}

			if matchTag && matchID {
				keepOps[op] = true
			}
		}
	}

	return keepOps, isFiltering
}

// getComponentCount returns the total number of top-level components.
func (c *cleaner) getComponentCount() int {
	comps := c.doc.Components
	if comps == nil {
		return 0
	}
	return len(comps.Schemas) + len(comps.RequestBodies) + len(comps.Responses) +
		len(comps.Parameters) + len(comps.Headers) + len(comps.Examples) +
		len(comps.SecuritySchemes) + len(comps.Links) + len(comps.Callbacks)
}

// lookupRef attempts to find the component Ref object for a given reference string.
func (c *cleaner) lookupRef(ref string) interface{} {
	if c.doc.Components == nil {
		return nil
	}

	const (
		schemasPrefix       = "#/components/schemas/"
		responsesPrefix     = "#/components/responses/"
		parametersPrefix    = "#/components/parameters/"
		requestBodiesPrefix = "#/components/requestBodies/"
		headersPrefix       = "#/components/headers/"
		examplesPrefix      = "#/components/examples/"
		securityPrefix      = "#/components/securitySchemes/"
		linksPrefix         = "#/components/links/"
		callbacksPrefix     = "#/components/callbacks/"
	)

	switch {
	case strings.HasPrefix(ref, schemasPrefix):
		name := strings.TrimPrefix(ref, schemasPrefix)
		if r, ok := c.doc.Components.Schemas[name]; ok {
			return r
		}
	case strings.HasPrefix(ref, responsesPrefix):
		name := strings.TrimPrefix(ref, responsesPrefix)
		if r, ok := c.doc.Components.Responses[name]; ok {
			return r
		}
	case strings.HasPrefix(ref, parametersPrefix):
		name := strings.TrimPrefix(ref, parametersPrefix)
		if r, ok := c.doc.Components.Parameters[name]; ok {
			return r
		}
	case strings.HasPrefix(ref, requestBodiesPrefix):
		name := strings.TrimPrefix(ref, requestBodiesPrefix)
		if r, ok := c.doc.Components.RequestBodies[name]; ok {
			return r
		}
	case strings.HasPrefix(ref, headersPrefix):
		name := strings.TrimPrefix(ref, headersPrefix)
		if r, ok := c.doc.Components.Headers[name]; ok {
			return r
		}
	case strings.HasPrefix(ref, examplesPrefix):
		name := strings.TrimPrefix(ref, examplesPrefix)
		if r, ok := c.doc.Components.Examples[name]; ok {
			return r
		}
	case strings.HasPrefix(ref, securityPrefix):
		name := strings.TrimPrefix(ref, securityPrefix)
		if r, ok := c.doc.Components.SecuritySchemes[name]; ok {
			return r
		}
	case strings.HasPrefix(ref, linksPrefix):
		name := strings.TrimPrefix(ref, linksPrefix)
		if r, ok := c.doc.Components.Links[name]; ok {
			return r
		}
	case strings.HasPrefix(ref, callbacksPrefix):
		name := strings.TrimPrefix(ref, callbacksPrefix)
		if r, ok := c.doc.Components.Callbacks[name]; ok {
			return r
		}
	}
	return nil
}
