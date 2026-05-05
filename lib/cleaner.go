// Package lib provides functionality to clean and prune OpenAPI 3 specifications.
// It can selectively remove extensions, examples, non-2xx responses, and unreferenced components
// while preserving the core structure and descriptions.
package lib

import (
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
)

// CleanOptions defines the configuration for the cleaning process.
type CleanOptions struct {
	// CleanExamples, if true, removes 'example' and 'examples' fields from all components.
	CleanExamples bool
	// KeepOperationIDs, if non-empty, specifies the only operation IDs to be kept in the spec.
	// Operations not in this list will be removed, and empty paths will be pruned.
	KeepOperationIDs []string
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

// cleaner holds the internal state for a single cleaning run.
// State encapsulation prevents side effects and allows for easier tracking of visited nodes
// in recursive operations, which is critical for handling circular references.
type cleaner struct {
	doc     *openapi3.T
	opts    CleanOptions
	keepOps map[string]bool
	// visitedSchemas tracks pointers to openapi3.Schema objects to prevent infinite recursion
	// during the cleaning phase.
	visitedSchemas map[*openapi3.Schema]bool
	// visitedObjects tracks generic pointers during the tracing phase (GC) to prevent
	// infinite recursion when following references.
	visitedObjects map[interface{}]bool
}

// CleanAndPruneOpenAPI is the main entry point for the cleaning process.
// It modifies the provided openapi3.T document in-place based on the CleanOptions.
func CleanAndPruneOpenAPI(doc *openapi3.T, opts CleanOptions) {
	keepOps := make(map[string]bool, len(opts.KeepOperationIDs))
	for _, id := range opts.KeepOperationIDs {
		keepOps[id] = true
	}

	c := &cleaner{
		doc:            doc,
		opts:           opts,
		keepOps:        keepOps,
		visitedSchemas: make(map[*openapi3.Schema]bool),
		visitedObjects: make(map[interface{}]bool),
	}

	c.cleanMetadata()
	c.cleanPaths()
	c.cleanTags()
	// pruneComponents implements a mark-and-sweep style garbage collector to remove
	// top-level components that are no longer reachable from any operation.
	c.pruneComponents()
}

// cleanMetadata removes top-level document metadata like contact, license, and extensions.
func (c *cleaner) cleanMetadata() {
	if c.opts.RemoveExtensions {
		c.doc.Extensions = nil
	}

	if c.opts.StripMetadata {
		c.doc.ExternalDocs = nil
		if c.doc.Info != nil {
			if c.opts.RemoveExtensions {
				c.doc.Info.Extensions = nil
			}
			c.doc.Info.TermsOfService = ""
			c.doc.Info.Contact = nil
			c.doc.Info.License = nil
		}
	} else if c.doc.Info != nil && c.opts.RemoveExtensions {
		// Even if not stripping metadata, we still honor RemoveExtensions inside Info
		c.doc.Info.Extensions = nil
	}
}

// cleanPaths iterates through all API paths and operations, applying filters and cleaning logic.
func (c *cleaner) cleanPaths() {
	if c.doc.Paths == nil {
		return
	}

	if c.opts.RemoveExtensions {
		c.doc.Paths.Extensions = nil
	}
	for path, pathItem := range c.doc.Paths.Map() {
		if pathItem == nil {
			continue
		}
		if c.opts.RemoveExtensions {
			pathItem.Extensions = nil
		}

		// Filter operations by operationId. This is done before cleaning individual operations
		// to avoid unnecessary work on operations that will be discarded.
		if len(c.keepOps) > 0 {
			for method, op := range pathItem.Operations() {
				if op.OperationID == "" || !c.keepOps[op.OperationID] {
					pathItem.SetOperation(method, nil)
				}
			}
			// Remove the entire path if it has no operations left after filtering.
			if len(pathItem.Operations()) == 0 {
				c.doc.Paths.Delete(path)
				continue
			}
		}

		for _, op := range pathItem.Operations() {
			c.cleanOperation(op)
		}
	}
}

// cleanOperation handles the cleaning of a single API operation, including its responses and bodies.
func (c *cleaner) cleanOperation(op *openapi3.Operation) {
	if c.opts.StripMetadata {
		op.ExternalDocs = nil
	}
	if c.opts.RemoveExtensions {
		op.Extensions = nil
	}
	op.Deprecated = false

	// Filter responses by status code. Only 2xx responses are kept if RemoveNon2xxErrors is set.
	if c.opts.RemoveNon2xxErrors {
		for status := range op.Responses.Map() {
			if len(status) > 0 && status[0] != '2' {
				op.Responses.Delete(status)
			}
		}
		op.Responses.Delete("default")
	}

	for _, respRef := range op.Responses.Map() {
		c.cleanResponse(respRef)
	}
	if def := op.Responses.Default(); def != nil {
		c.cleanResponse(def)
	}

	if op.RequestBody != nil && op.RequestBody.Value != nil {
		if c.opts.RemoveExtensions {
			op.RequestBody.Value.Extensions = nil
		}
		c.cleanContent(op.RequestBody.Value.Content)
	}

	for _, p := range op.Parameters {
		if p.Value != nil {
			if c.opts.RemoveExtensions {
				p.Value.Extensions = nil
			}
			c.cleanExampleAndSchema(&p.Value.Example, p.Value.Examples, p.Value.Schema)
		}
	}
}

// cleanResponse cleans a single response object.
func (c *cleaner) cleanResponse(respRef *openapi3.ResponseRef) {
	if respRef == nil || respRef.Value == nil {
		return
	}

	res := respRef.Value
	if c.opts.RemoveExtensions {
		res.Extensions = nil
	}

	for _, header := range res.Headers {
		if header.Value != nil {
			if c.opts.RemoveExtensions {
				header.Value.Extensions = nil
			}
			c.cleanExampleAndSchema(&header.Value.Example, header.Value.Examples, header.Value.Schema)
		}
	}

	if c.opts.RemoveResponseSchemas {
		res.Content = nil
	} else {
		c.cleanContent(res.Content)
	}
}

// cleanContent cleans a media type map (Content).
func (c *cleaner) cleanContent(content openapi3.Content) {
	for _, mt := range content {
		if c.opts.RemoveExtensions {
			mt.Extensions = nil
		}
		c.cleanExampleAndSchema(&mt.Example, mt.Examples, mt.Schema)
	}
}

// cleanExampleAndSchema is a helper to unify the cleaning of common fields in Parameters, Headers, and Content.
func (c *cleaner) cleanExampleAndSchema(example *interface{}, examples openapi3.Examples, schema *openapi3.SchemaRef) {
	if c.opts.CleanExamples {
		if example != nil {
			*example = nil
		}
		for k := range examples {
			delete(examples, k)
		}
	}
	if schema != nil {
		c.cleanSchema(schema.Value)
	}
}

// cleanSchema recursively cleans a schema object.
// It uses visitedSchemas to prevent infinite loops in recursive data structures.
func (c *cleaner) cleanSchema(s *openapi3.Schema) {
	if s == nil || c.visitedSchemas[s] {
		return
	}
	c.visitedSchemas[s] = true

	if c.opts.CleanExamples {
		s.Example = nil
	}
	if c.opts.RemoveExtensions {
		s.Extensions = nil
	}
	if c.opts.StripMetadata {
		s.ExternalDocs = nil
	}
	s.Deprecated = false

	// Recursive traversal of sub-schemas in properties and composition keywords (allOf, anyOf, etc.)
	for _, prop := range s.Properties {
		c.cleanSchema(prop.Value)
	}
	if s.Items != nil {
		c.cleanSchema(s.Items.Value)
	}
	for _, sub := range s.AllOf {
		c.cleanSchema(sub.Value)
	}
	for _, sub := range s.AnyOf {
		c.cleanSchema(sub.Value)
	}
	for _, sub := range s.OneOf {
		c.cleanSchema(sub.Value)
	}
	if s.AdditionalProperties.Schema != nil {
		c.cleanSchema(s.AdditionalProperties.Schema.Value)
	}
}

// cleanTags removes tags that are no longer referenced by any operation.
func (c *cleaner) cleanTags() {
	if c.doc.Tags == nil {
		return
	}

	usedTags := make(map[string]bool)
	if len(c.keepOps) > 0 {
		for _, pathItem := range c.doc.Paths.Map() {
			for _, op := range pathItem.Operations() {
				for _, tag := range op.Tags {
					usedTags[tag] = true
				}
			}
		}
	} else {
		// If we are not filtering operations, all tags are considered "used" for the purpose of cleaning them.
		for _, tag := range c.doc.Tags {
			usedTags[tag.Name] = true
		}
	}

	filtered := make(openapi3.Tags, 0, len(c.doc.Tags))
	for _, tag := range c.doc.Tags {
		if usedTags[tag.Name] {
			if c.opts.StripMetadata {
				tag.ExternalDocs = nil
			}
			if c.opts.RemoveExtensions {
				tag.Extensions = nil
			}
			filtered = append(filtered, tag)
		}
	}
	c.doc.Tags = filtered
}

// pruneComponents removes unreferenced top-level components from the document.
// Since components can reference each other, this process is run in a loop until no more
// components can be pruned (fixed-point iteration).
func (c *cleaner) pruneComponents() {
	if c.doc.Components == nil {
		return
	}

	if c.opts.RemoveExtensions {
		c.doc.Components.Extensions = nil
	}
	if c.opts.CleanExamples {
		c.doc.Components.Examples = nil
	}

	if c.opts.RemoveResponseSchemas {
		for _, respRef := range c.doc.Components.Responses {
			respRef.Ref = ""
			if respRef.Value != nil {
				respRef.Value.Content = nil
			}
		}
	}

	for {
		usedRefs := make(map[string]bool)
		c.visitedObjects = make(map[interface{}]bool) // Reset visited objects for each trace
		c.traceAllRefs(usedRefs)

		initialLen := c.getComponentCount()

		// Prune Unused components from each category.
		for name := range c.doc.Components.Schemas {
			if !usedRefs["#/components/schemas/"+name] {
				delete(c.doc.Components.Schemas, name)
			} else {
				// We still need to clean the schemas that are kept.
				c.cleanSchema(c.doc.Components.Schemas[name].Value)
			}
		}
		for name := range c.doc.Components.RequestBodies {
			if !usedRefs["#/components/requestBodies/"+name] {
				delete(c.doc.Components.RequestBodies, name)
			}
		}
		for name := range c.doc.Components.Responses {
			if !usedRefs["#/components/responses/"+name] {
				delete(c.doc.Components.Responses, name)
			}
		}
		for name := range c.doc.Components.Parameters {
			if !usedRefs["#/components/parameters/"+name] {
				delete(c.doc.Components.Parameters, name)
			}
		}
		for name := range c.doc.Components.Headers {
			if !usedRefs["#/components/headers/"+name] {
				delete(c.doc.Components.Headers, name)
			}
		}

		// If no components were removed in this pass, we've reached a stable state.
		if c.getComponentCount() == initialLen {
			break
		}
	}
}

// traceAllRefs explores the document to identify all reachable components.
func (c *cleaner) traceAllRefs(used map[string]bool) {
	for _, pathItem := range c.doc.Paths.Map() {
		for _, op := range pathItem.Operations() {
			for _, p := range op.Parameters {
				c.traceRefs(p.Ref, p.Value, used)
			}
			if op.RequestBody != nil {
				c.traceRefs(op.RequestBody.Ref, op.RequestBody.Value, used)
			}
			for _, resp := range op.Responses.Map() {
				c.traceRefs(resp.Ref, resp.Value, used)
			}
			if def := op.Responses.Default(); def != nil {
				c.traceRefs(def.Ref, def.Value, used)
			}
		}
	}
}

// getComponentCount returns the total number of top-level components.
func (c *cleaner) getComponentCount() int {
	comps := c.doc.Components
	if comps == nil {
		return 0
	}
	return len(comps.Schemas) + len(comps.RequestBodies) + len(comps.Responses) +
		len(comps.Parameters) + len(comps.Headers) + len(comps.Examples)
}

// traceRefs recursively follows references and marks reachable components in the 'used' map.
// It uses reflection-based nil checks and visitedObjects to safely handle circular references
// and typed nil pointers.
func (c *cleaner) traceRefs(ref string, val interface{}, used map[string]bool) {
	if ref != "" {
		if used[ref] {
			return
		}
		used[ref] = true
	}

	// Nil check for both raw nil and typed nil interfaces (e.g., *SchemaRef(nil)).
	if val == nil || (reflect.ValueOf(val).Kind() == reflect.Ptr && reflect.ValueOf(val).IsNil()) {
		return
	}

	// If we are traversing an object directly (no $ref yet), we track it to prevent infinite loops.
	if ref == "" {
		if c.visitedObjects[val] {
			return
		}
		c.visitedObjects[val] = true
	}

	switch v := val.(type) {
	case *openapi3.Response:
		for _, header := range v.Headers {
			c.traceRefs(header.Ref, header.Value, used)
		}
		for _, mt := range v.Content {
			if mt.Schema != nil {
				c.traceRefs(mt.Schema.Ref, mt.Schema.Value, used)
			}
		}
	case *openapi3.RequestBody:
		for _, mt := range v.Content {
			if mt.Schema != nil {
				c.traceRefs(mt.Schema.Ref, mt.Schema.Value, used)
			}
		}
	case *openapi3.Parameter:
		if v.Schema != nil {
			c.traceRefs(v.Schema.Ref, v.Schema.Value, used)
		}
	case *openapi3.Header:
		if v.Schema != nil {
			c.traceRefs(v.Schema.Ref, v.Schema.Value, used)
		}
	case *openapi3.Schema:
		// Traverse all possible sub-schema references.
		for _, prop := range v.Properties {
			c.traceRefs(prop.Ref, prop.Value, used)
		}
		if v.Items != nil {
			c.traceRefs(v.Items.Ref, v.Items.Value, used)
		}
		for _, s := range v.AllOf {
			c.traceRefs(s.Ref, s.Value, used)
		}
		for _, s := range v.AnyOf {
			c.traceRefs(s.Ref, s.Value, used)
		}
		for _, s := range v.OneOf {
			c.traceRefs(s.Ref, s.Value, used)
		}
		if v.Not != nil {
			c.traceRefs(v.Not.Ref, v.Not.Value, used)
		}
		if v.Discriminator != nil {
			for _, mRef := range v.Discriminator.Mapping {
				c.traceRefs(mRef.Ref, mRef.Value, used)
			}
		}
		if v.AdditionalProperties.Schema != nil {
			c.traceRefs(v.AdditionalProperties.Schema.Ref, v.AdditionalProperties.Schema.Value, used)
		}
	}
}
