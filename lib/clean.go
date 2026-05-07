package lib

import "github.com/getkin/kin-openapi/openapi3"

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
