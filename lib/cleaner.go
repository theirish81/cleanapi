// Package lib provides functionality to clean and prune OpenAPI 3 specifications.
// It can selectively remove extensions, examples, non-2xx responses, and unreferenced components
// while preserving the core structure and descriptions.
package lib

import (
	"reflect"
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

// cleanServers removes extensions from a list of Server objects.
func (c *cleaner) cleanServers(servers openapi3.Servers) {
	if !c.opts.RemoveExtensions {
		return
	}
	for _, s := range servers {
		if s != nil {
			s.Extensions = nil
			for _, v := range s.Variables {
				if v != nil {
					v.Extensions = nil
				}
			}
		}
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

		c.cleanServers(pathItem.Servers)

		// Filter operations by operationId or tags. This is done before cleaning individual operations
		// to avoid unnecessary work on operations that will be discarded.
		if c.isFiltering {
			for method, op := range pathItem.Operations() {
				if !c.keepOps[op] {
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

// cleanOperation handles the cleaning of a single API operation.
func (c *cleaner) cleanOperation(op *openapi3.Operation) {
	if c.opts.StripMetadata {
		op.ExternalDocs = nil
	}
	if c.opts.RemoveExtensions {
		op.Extensions = nil
	}
	if op.Servers != nil {
		c.cleanServers(*op.Servers)
	}

	// Deprecated status is considered metadata that can be stripped if requested.
	if c.opts.StripMetadata {
		op.Deprecated = false
	}

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
		c.cleanRequestBody(op.RequestBody.Value)
	}

	for _, p := range op.Parameters {
		if p.Value != nil {
			c.cleanParameter(p.Value)
		}
	}
}

// cleanRequestBody cleans a RequestBody object.
func (c *cleaner) cleanRequestBody(rb *openapi3.RequestBody) {
	if rb == nil {
		return
	}
	if c.opts.RemoveExtensions {
		rb.Extensions = nil
	}
	c.cleanContent(rb.Content)
}

// cleanParameter cleans a Parameter object.
func (c *cleaner) cleanParameter(p *openapi3.Parameter) {
	if p == nil {
		return
	}
	if c.opts.RemoveExtensions {
		p.Extensions = nil
	}
	c.cleanExampleAndSchema(&p.Example, p.Examples, p.Schema)
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
	for _, h := range res.Headers {
		if h != nil && h.Value != nil {
			if c.opts.RemoveExtensions {
				h.Value.Extensions = nil
			}
			c.cleanExampleAndSchema(&h.Value.Example, h.Value.Examples, h.Value.Schema)
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

// cleanExampleAndSchema is a helper to unify the cleaning of common fields.
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
		s.Deprecated = false
	}

	// Recursive traversal of sub-schemas.
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
	if c.isFiltering {
		for _, pathItem := range c.doc.Paths.Map() {
			for _, op := range pathItem.Operations() {
				for _, tag := range op.Tags {
					usedTags[tag] = true
				}
			}
		}
	} else {
		// If we are not filtering operations, all tags are considered "used".
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

// pruneComponents removes unreferenced top-level components using mark-and-sweep.
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

	// If we are stripping all response schemas, we can clear them in the components section too.
	if c.opts.RemoveResponseSchemas {
		for _, respRef := range c.doc.Components.Responses {
			if respRef != nil && respRef.Value != nil {
				respRef.Value.Content = nil
			}
		}
	}

	usedRefs := make(map[string]bool)
	c.visitedObjects = make(map[interface{}]bool)
	c.traceAllRefs(usedRefs)

	// Prune unused components and clean the ones we keep.
	c.pruneAndCleanSchemas(usedRefs)
	c.pruneAndCleanCategory(c.doc.Components.RequestBodies, "#/components/requestBodies/", usedRefs, func(v interface{}) {
		c.cleanRequestBody(v.(*openapi3.RequestBody))
	})
	c.pruneAndCleanCategory(c.doc.Components.Responses, "#/components/responses/", usedRefs, func(v interface{}) {
		c.cleanResponse(&openapi3.ResponseRef{Value: v.(*openapi3.Response)})
	})
	c.pruneAndCleanCategory(c.doc.Components.Parameters, "#/components/parameters/", usedRefs, func(v interface{}) {
		c.cleanParameter(v.(*openapi3.Parameter))
	})
	c.pruneAndCleanCategory(c.doc.Components.Headers, "#/components/headers/", usedRefs, func(v interface{}) {
		h := v.(*openapi3.Header)
		if c.opts.RemoveExtensions {
			h.Extensions = nil
		}
		c.cleanExampleAndSchema(&h.Example, h.Examples, h.Schema)
	})
	c.pruneAndCleanCategory(c.doc.Components.Examples, "#/components/examples/", usedRefs, nil)
	c.pruneAndCleanCategory(c.doc.Components.SecuritySchemes, "#/components/securitySchemes/", usedRefs, func(v interface{}) {
		ss := v.(*openapi3.SecurityScheme)
		if c.opts.RemoveExtensions {
			ss.Extensions = nil
		}
	})
	c.pruneAndCleanCategory(c.doc.Components.Links, "#/components/links/", usedRefs, func(v interface{}) {
		l := v.(*openapi3.Link)
		if c.opts.RemoveExtensions {
			l.Extensions = nil
		}
	})
	c.pruneAndCleanCategory(c.doc.Components.Callbacks, "#/components/callbacks/", usedRefs, nil)
}

// pruneAndCleanSchemas handles the Schemas category specifically due to recursive cleaning.
func (c *cleaner) pruneAndCleanSchemas(used map[string]bool) {
	for name, ref := range c.doc.Components.Schemas {
		if !used["#/components/schemas/"+name] {
			delete(c.doc.Components.Schemas, name)
		} else if ref != nil {
			c.cleanSchema(ref.Value)
		}
	}
}

// pruneAndCleanCategory handles generic component categories.
func (c *cleaner) pruneAndCleanCategory(m interface{}, prefix string, used map[string]bool, cleanFn func(interface{})) {
	v := reflect.ValueOf(m)
	if v.Kind() != reflect.Map {
		return
	}
	for _, key := range v.MapKeys() {
		name := key.String()
		if !used[prefix+name] {
			v.SetMapIndex(key, reflect.Value{})
		} else if cleanFn != nil {
			elem := v.MapIndex(key)
			if !elem.IsNil() {
				valField := elem.Elem().FieldByName("Value")
				if valField.IsValid() && !valField.IsNil() {
					cleanFn(valField.Interface())
				}
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
		len(comps.Parameters) + len(comps.Headers) + len(comps.Examples) +
		len(comps.SecuritySchemes) + len(comps.Links) + len(comps.Callbacks)
}

// traceAllRefs explores the document to identify all reachable components.
func (c *cleaner) traceAllRefs(used map[string]bool) {
	// Global security requirements
	for _, sec := range c.doc.Security {
		for name := range sec {
			used["#/components/securitySchemes/"+name] = true
		}
	}

	if c.doc.Paths == nil {
		return
	}

	for _, pathItem := range c.doc.Paths.Map() {
		if pathItem == nil {
			continue
		}
		if pathItem.Ref != "" {
			used[pathItem.Ref] = true
		}
		for _, p := range pathItem.Parameters {
			c.traceRefs(p.Ref, p.Value, used)
		}

		for _, op := range pathItem.Operations() {
			c.traceOperationRefs(op, used)
		}
	}
}

// traceOperationRefs traces all references in a single operation.
func (c *cleaner) traceOperationRefs(op *openapi3.Operation, used map[string]bool) {
	if op == nil {
		return
	}
	if op.Security != nil {
		for _, sec := range *op.Security {
			for name := range sec {
				used["#/components/securitySchemes/"+name] = true
			}
		}
	}

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

	for _, callback := range op.Callbacks {
		if callback != nil {
			c.traceRefs(callback.Ref, callback.Value, used)
		}
	}
}

// traceMediaTypeRefs traces all references in a media type object.
func (c *cleaner) traceMediaTypeRefs(mt *openapi3.MediaType, used map[string]bool) {
	if mt == nil {
		return
	}
	if mt.Schema != nil {
		c.traceRefs(mt.Schema.Ref, mt.Schema.Value, used)
	}
	for _, ex := range mt.Examples {
		c.traceRefs(ex.Ref, ex.Value, used)
	}
}

// isNil is a robust nil check for interfaces including typed nil pointers.
func isNil(i interface{}) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	if v.Kind() == reflect.Ptr {
		return v.IsNil()
	}
	return false
}

// traceRefs recursively follows references and marks reachable components.
func (c *cleaner) traceRefs(ref string, val interface{}, used map[string]bool) {
	if ref != "" {
		if used[ref] {
			return
		}
		used[ref] = true
	}

	if isNil(val) {
		if ref != "" {
			val = c.lookupRef(ref)
			if isNil(val) {
				return
			}
		} else {
			return
		}
	}

	// Prevent infinite loops in recursive structures.
	if ref == "" {
		if c.visitedObjects[val] {
			return
		}
		c.visitedObjects[val] = true
	}

	switch v := val.(type) {
	case *openapi3.SchemaRef:
		c.traceRefs(v.Ref, v.Value, used)
	case *openapi3.ResponseRef:
		c.traceRefs(v.Ref, v.Value, used)
	case *openapi3.RequestBodyRef:
		c.traceRefs(v.Ref, v.Value, used)
	case *openapi3.ParameterRef:
		c.traceRefs(v.Ref, v.Value, used)
	case *openapi3.HeaderRef:
		c.traceRefs(v.Ref, v.Value, used)
	case *openapi3.ExampleRef:
		c.traceRefs(v.Ref, v.Value, used)
	case *openapi3.SecuritySchemeRef:
		c.traceRefs(v.Ref, v.Value, used)
	case *openapi3.LinkRef:
		c.traceRefs(v.Ref, v.Value, used)
	case *openapi3.CallbackRef:
		c.traceRefs(v.Ref, v.Value, used)

	case *openapi3.Response:
		for _, h := range v.Headers {
			c.traceRefs(h.Ref, h.Value, used)
		}
		for _, mt := range v.Content {
			c.traceMediaTypeRefs(mt, used)
		}
		for _, l := range v.Links {
			c.traceRefs(l.Ref, l.Value, used)
		}
	case *openapi3.RequestBody:
		for _, mt := range v.Content {
			c.traceMediaTypeRefs(mt, used)
		}
	case *openapi3.Parameter:
		if v.Schema != nil {
			c.traceRefs(v.Schema.Ref, v.Schema.Value, used)
		}
		for _, ex := range v.Examples {
			c.traceRefs(ex.Ref, ex.Value, used)
		}
	case *openapi3.Header:
		if v.Schema != nil {
			c.traceRefs(v.Schema.Ref, v.Schema.Value, used)
		}
		for _, ex := range v.Examples {
			c.traceRefs(ex.Ref, ex.Value, used)
		}
	case *openapi3.Schema:
		c.traceSchemaRefs(v, used)
	case *openapi3.Link:
		// OperationRef is tracked if we implement operation component tracking.
	case *openapi3.Callback:
		for _, pathItem := range v.Map() {
			if pathItem != nil {
				for _, op := range pathItem.Operations() {
					c.traceOperationRefs(op, used)
				}
			}
		}
	case *openapi3.Example:
		// Leaf node.
	case *openapi3.SecurityScheme:
		// Usually leaf nodes regarding component refs.
	}
}

// traceSchemaRefs traces all references within a schema.
func (c *cleaner) traceSchemaRefs(s *openapi3.Schema, used map[string]bool) {
	for _, prop := range s.Properties {
		c.traceRefs(prop.Ref, prop.Value, used)
	}
	if s.Items != nil {
		c.traceRefs(s.Items.Ref, s.Items.Value, used)
	}
	for _, sub := range s.AllOf {
		c.traceRefs(sub.Ref, sub.Value, used)
	}
	for _, sub := range s.AnyOf {
		c.traceRefs(sub.Ref, sub.Value, used)
	}
	for _, sub := range s.OneOf {
		c.traceRefs(sub.Ref, sub.Value, used)
	}
	if s.Not != nil {
		c.traceRefs(s.Not.Ref, s.Not.Value, used)
	}
	if s.Discriminator != nil {
		for _, mRef := range s.Discriminator.Mapping {
			c.traceRefs(mRef.Ref, mRef.Value, used)
		}
	}
	if s.AdditionalProperties.Schema != nil {
		c.traceRefs(s.AdditionalProperties.Schema.Ref, s.AdditionalProperties.Schema.Value, used)
	}
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
