package lib

import (
	"github.com/getkin/kin-openapi/openapi3"
)

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
