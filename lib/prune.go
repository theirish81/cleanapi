package lib

import (
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
)

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
