package lib

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
)

// Helper to create string pointers
func ptrString(s string) *string {
	return &s
}

// TestCleanAndPruneOpenAPI_PruneUnusedComponents verifies that top-level components
// (like schemas) that are not reachable from any path are removed.
func TestCleanAndPruneOpenAPI_PruneUnusedComponents(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: make(openapi3.Schemas),
		},
	}

	usedSchema := &openapi3.Schema{Type: &openapi3.Types{"string"}}
	unusedSchema := &openapi3.Schema{Type: &openapi3.Types{"integer"}}

	doc.Components.Schemas["Used"] = &openapi3.SchemaRef{Value: usedSchema}
	doc.Components.Schemas["Unused"] = &openapi3.SchemaRef{Value: unusedSchema}

	op := &openapi3.Operation{
		Responses: openapi3.NewResponses(),
	}
	op.Responses.Set("200", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/Used"}),
		},
	})
	doc.Paths.Set("/test", &openapi3.PathItem{Get: op})

	CleanAndPruneOpenAPI(doc, CleanOptions{})

	_, okUsed := doc.Components.Schemas["Used"]
	_, okUnused := doc.Components.Schemas["Unused"]

	assert.True(t, okUsed, "expected 'Used' schema to be kept")
	assert.False(t, okUnused, "expected 'Unused' schema to be pruned")
}

// TestCleanAndPruneOpenAPI_RecursiveSchema ensures the reachability engine handles
// circular schema references without entering an infinite loop or crashing.
func TestCleanAndPruneOpenAPI_RecursiveSchema(t *testing.T) {
	// Create a recursive schema: User -> friend -> User
	userSchema := &openapi3.Schema{
		Type:       &openapi3.Types{"object"},
		Properties: make(openapi3.Schemas),
	}
	userSchema.Properties["friend"] = &openapi3.SchemaRef{
		Value: userSchema,
	}

	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Recursive API", Version: "1.0.0"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: make(openapi3.Schemas),
		},
	}
	doc.Components.Schemas["User"] = &openapi3.SchemaRef{Value: userSchema}

	op := &openapi3.Operation{
		OperationID: "getUser",
		Responses:   openapi3.NewResponses(),
	}
	op.Responses.Set("200", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/User"}),
		},
	})
	doc.Paths.Set("/user", &openapi3.PathItem{Get: op})

	// This should not hang or crash
	assert.NotPanics(t, func() {
		CleanAndPruneOpenAPI(doc, CleanOptions{})
	})
	assert.Contains(t, doc.Components.Schemas, "User")
}

// TestCleanAndPruneOpenAPI_AllOf verifies that schemas referenced inside an 'allOf'
// composition are correctly traced as "used".
func TestCleanAndPruneOpenAPI_AllOf(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: make(openapi3.Schemas),
		},
	}

	doc.Components.Schemas["Base"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{Type: &openapi3.Types{"object"}},
	}
	doc.Components.Schemas["Extended"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			AllOf: openapi3.SchemaRefs{{Ref: "#/components/schemas/Base"}},
		},
	}

	op := &openapi3.Operation{
		Responses: openapi3.NewResponses(),
	}
	op.Responses.Set("200", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/Extended"}),
		},
	})
	doc.Paths.Set("/test", &openapi3.PathItem{Get: op})

	CleanAndPruneOpenAPI(doc, CleanOptions{})

	assert.Contains(t, doc.Components.Schemas, "Extended")
	assert.Contains(t, doc.Components.Schemas, "Base")
}

// TestCleanAndPruneOpenAPI_OneOfAnyOf verifies that schemas referenced inside 'oneOf'
// or 'anyOf' compositions are correctly traced as "used".
func TestCleanAndPruneOpenAPI_OneOfAnyOf(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: make(openapi3.Schemas),
		},
	}

	doc.Components.Schemas["OptionA"] = &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}}
	doc.Components.Schemas["OptionB"] = &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}}

	doc.Components.Schemas["Union"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			OneOf: openapi3.SchemaRefs{{Ref: "#/components/schemas/OptionA"}},
			AnyOf: openapi3.SchemaRefs{{Ref: "#/components/schemas/OptionB"}},
		},
	}

	op := &openapi3.Operation{
		Responses: openapi3.NewResponses(),
	}
	op.Responses.Set("200", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/Union"}),
		},
	})
	doc.Paths.Set("/test", &openapi3.PathItem{Get: op})

	CleanAndPruneOpenAPI(doc, CleanOptions{})

	assert.Contains(t, doc.Components.Schemas, "Union")
	assert.Contains(t, doc.Components.Schemas, "OptionA")
	assert.Contains(t, doc.Components.Schemas, "OptionB")
}

// TestCleanAndPruneOpenAPI_Security verifies that Security Schemes used in the root
// of the document are preserved.
func TestCleanAndPruneOpenAPI_Security(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			SecuritySchemes: make(openapi3.SecuritySchemes),
		},
	}

	doc.Components.SecuritySchemes["UsedScheme"] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{Type: "apiKey"},
	}
	doc.Components.SecuritySchemes["UnusedScheme"] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{Type: "http"},
	}

	doc.Security = openapi3.SecurityRequirements{
		{"UsedScheme": []string{}},
	}

	CleanAndPruneOpenAPI(doc, CleanOptions{})

	assert.Contains(t, doc.Components.SecuritySchemes, "UsedScheme")
	assert.NotContains(t, doc.Components.SecuritySchemes, "UnusedScheme")
}

// TestCleanAndPruneOpenAPI_NestedRequestBody verifies that if a RequestBody component
// references another RequestBody component, both are preserved (deep $ref tracing).
func TestCleanAndPruneOpenAPI_NestedRequestBody(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			RequestBodies: make(map[string]*openapi3.RequestBodyRef),
		},
	}

	doc.Components.RequestBodies["B"] = &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{Description: "B"},
	}
	doc.Components.RequestBodies["A"] = &openapi3.RequestBodyRef{
		Ref: "#/components/requestBodies/B",
	}

	op := &openapi3.Operation{
		RequestBody: &openapi3.RequestBodyRef{Ref: "#/components/requestBodies/A"},
		Responses:   openapi3.NewResponses(),
	}
	op.Responses.Set("200", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptrString("OK")}})
	doc.Paths.Set("/test", &openapi3.PathItem{Post: op})

	CleanAndPruneOpenAPI(doc, CleanOptions{})

	assert.Contains(t, doc.Components.RequestBodies, "A")
	assert.Contains(t, doc.Components.RequestBodies, "B")
}

// TestCleanAndPruneOpenAPI_KeepOperationIDs verifies that only explicitly listed
// operation IDs are kept and others are removed.
func TestCleanAndPruneOpenAPI_KeepOperationIDs(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Paths:   openapi3.NewPaths(),
	}

	op1 := &openapi3.Operation{OperationID: "keepMe"}
	op2 := &openapi3.Operation{OperationID: "removeMe"}

	doc.Paths.Set("/keep", &openapi3.PathItem{Get: op1})
	doc.Paths.Set("/remove", &openapi3.PathItem{Get: op2})

	opts := CleanOptions{
		KeepOperationIDs: []string{"keepMe"},
	}

	CleanAndPruneOpenAPI(doc, opts)

	assert.NotNil(t, doc.Paths.Find("/keep"))
	assert.Nil(t, doc.Paths.Find("/remove"))
}

// TestCleanAndPruneOpenAPI_KeepTags verifies that operations can be filtered by tag,
// and that combining IDs and Tags uses INTERSECTION (AND) logic.
func TestCleanAndPruneOpenAPI_KeepTags(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Paths:   openapi3.NewPaths(),
	}

	op1 := &openapi3.Operation{OperationID: "op1", Tags: []string{"tag1"}}
	op2 := &openapi3.Operation{OperationID: "op2", Tags: []string{"tag2"}}
	op3 := &openapi3.Operation{OperationID: "op3", Tags: []string{"tag1", "tag3"}}
	op4 := &openapi3.Operation{OperationID: "op4", Tags: []string{"tag4"}}

	doc.Paths.Set("/path1", &openapi3.PathItem{Get: op1})
	doc.Paths.Set("/path2", &openapi3.PathItem{Get: op2})
	doc.Paths.Set("/path3", &openapi3.PathItem{Get: op3})
	doc.Paths.Set("/path4", &openapi3.PathItem{Get: op4})

	// 1. Filter by tag only
	opts := CleanOptions{KeepTags: []string{"tag1"}}
	CleanAndPruneOpenAPI(doc, opts)
	assert.NotNil(t, doc.Paths.Find("/path1"))
	assert.Nil(t, doc.Paths.Find("/path2"))
	assert.NotNil(t, doc.Paths.Find("/path3"))

	// 2. Intersection logic: Tag AND ID
	doc.Paths = openapi3.NewPaths() // Reset
	doc.Paths.Set("/path1", &openapi3.PathItem{Get: op1})
	doc.Paths.Set("/path2", &openapi3.PathItem{Get: op2})
	doc.Paths.Set("/path3", &openapi3.PathItem{Get: op3})

	opts = CleanOptions{
		KeepTags:         []string{"tag1"},
		KeepOperationIDs: []string{"op3"},
	}

	CleanAndPruneOpenAPI(doc, opts)

	assert.Nil(t, doc.Paths.Find("/path1"))    // tag matches but ID doesn't
	assert.NotNil(t, doc.Paths.Find("/path3")) // both match
}

// TestCleanAndPruneOpenAPI_CleanExamples verifies that example data is stripped
// from schemas, parameters, and headers when CleanExamples is true.
func TestCleanAndPruneOpenAPI_CleanExamples(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Paths:   openapi3.NewPaths(),
	}

	schema := &openapi3.Schema{
		Type:    &openapi3.Types{"string"},
		Example: "my-example",
	}
	param := &openapi3.Parameter{
		Name:    "myParam",
		In:      "query",
		Schema:  &openapi3.SchemaRef{Value: schema},
		Example: "param-example",
	}

	op := &openapi3.Operation{
		Parameters: openapi3.Parameters{{Value: param}},
		Responses:  openapi3.NewResponses(),
	}
	op.Responses.Set("200", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptrString("OK")}})
	doc.Paths.Set("/test", &openapi3.PathItem{Get: op})

	opts := CleanOptions{CleanExamples: true}
	CleanAndPruneOpenAPI(doc, opts)

	assert.Nil(t, schema.Example)
	assert.Nil(t, param.Example)
}

// TestCleanAndPruneOpenAPI_RemoveNon2xx verifies that status codes not starting
// with '2' (and the 'default' response) are removed when RemoveNon2xxErrors is true.
func TestCleanAndPruneOpenAPI_RemoveNon2xx(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Paths:   openapi3.NewPaths(),
	}

	responses := openapi3.NewResponses()
	responses.Set("200", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptrString("OK")}})
	responses.Set("404", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptrString("Not Found")}})
	responses.Set("default", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptrString("Default")}})

	doc.Paths.Set("/test", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Responses: responses,
		},
	})

	opts := CleanOptions{RemoveNon2xxErrors: true}
	CleanAndPruneOpenAPI(doc, opts)

	op := doc.Paths.Find("/test").Get
	assert.NotNil(t, op.Responses.Value("200"))
	assert.Nil(t, op.Responses.Value("404"))
	assert.Nil(t, op.Responses.Default())
}

// TestCleanAndPruneOpenAPI_RemoveResponseSchemas verifies that the 'content' field
// is removed from response objects when RemoveResponseSchemas is true.
func TestCleanAndPruneOpenAPI_RemoveResponseSchemas(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Paths:   openapi3.NewPaths(),
	}

	responses := openapi3.NewResponses()
	responses.Set("200", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: ptrString("OK"),
			Content:     openapi3.NewContentWithJSONSchema(&openapi3.Schema{Type: &openapi3.Types{"string"}}),
		},
	})

	doc.Paths.Set("/test", &openapi3.PathItem{Get: &openapi3.Operation{Responses: responses}})

	opts := CleanOptions{RemoveResponseSchemas: true}
	CleanAndPruneOpenAPI(doc, opts)

	assert.Nil(t, responses.Value("200").Value.Content)
}

// TestCleanAndPruneOpenAPI_RemoveExtensions verifies that all 'x-' extensions
// are stripped document-wide when RemoveExtensions is true.
func TestCleanAndPruneOpenAPI_RemoveExtensions(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:      "Test API",
			Version:    "1.0.0",
			Extensions: map[string]interface{}{"x-info": "test"},
		},
		Extensions: map[string]interface{}{"x-root": "test"},
		Paths:      openapi3.NewPaths(),
	}

	pathItem := &openapi3.PathItem{
		Extensions: map[string]interface{}{"x-path": "test"},
		Get: &openapi3.Operation{
			Extensions: map[string]interface{}{"x-op": "test"},
			Responses:  openapi3.NewResponses(),
		},
	}
	doc.Paths.Set("/test", pathItem)

	opts := CleanOptions{RemoveExtensions: true}
	CleanAndPruneOpenAPI(doc, opts)

	assert.Nil(t, doc.Extensions)
	assert.Nil(t, doc.Info.Extensions)
	assert.Nil(t, pathItem.Extensions)
	assert.Nil(t, pathItem.Get.Extensions)
}

// TestCleanAndPruneOpenAPI_StripMetadata verifies that root-level metadata
// (Contact, License, ExternalDocs) and the 'deprecated' flag are removed.
func TestCleanAndPruneOpenAPI_StripMetadata(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:   "Test API",
			Version: "1.0.0",
			Contact: &openapi3.Contact{Name: "Test"},
			License: &openapi3.License{Name: "MIT"},
		},
		ExternalDocs: &openapi3.ExternalDocs{URL: "http://example.com"},
		Paths:        openapi3.NewPaths(),
	}

	op := &openapi3.Operation{
		Deprecated: true,
		Responses:  openapi3.NewResponses(),
	}
	doc.Paths.Set("/test", &openapi3.PathItem{Get: op})

	opts := CleanOptions{StripMetadata: true}
	CleanAndPruneOpenAPI(doc, opts)

	assert.Nil(t, doc.ExternalDocs)
	assert.Nil(t, doc.Info.Contact)
	assert.Nil(t, doc.Info.License)
	assert.False(t, op.Deprecated)
}
