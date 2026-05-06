package lib

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
)

func ptrString(s string) *string {
	return &s
}

func TestCleanAndPruneOpenAPI_Metadata(t *testing.T) {
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:   "Test API",
			Version: "1.0.0",
			Extensions: map[string]interface{}{
				"x-info-ext": "test",
			},
			Contact: &openapi3.Contact{Name: "Test"},
			License: &openapi3.License{Name: "MIT"},
		},
		Extensions: map[string]interface{}{
			"x-doc-ext": "test",
		},
		ExternalDocs: &openapi3.ExternalDocs{URL: "http://example.com"},
	}

	opts := CleanOptions{
		RemoveExtensions: true,
		StripMetadata:    true,
	}

	CleanAndPruneOpenAPI(doc, opts)

	assert.Nil(t, doc.Extensions)
	assert.Nil(t, doc.ExternalDocs)
	assert.Nil(t, doc.Info.Extensions)
	assert.Nil(t, doc.Info.Contact)
	assert.Nil(t, doc.Info.License)
}

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

	opts := CleanOptions{
		RemoveNon2xxErrors: true,
	}

	CleanAndPruneOpenAPI(doc, opts)

	op := doc.Paths.Find("/test").Get
	assert.NotNil(t, op.Responses.Value("200"))
	assert.Nil(t, op.Responses.Value("404"))
	assert.Nil(t, op.Responses.Default())
}

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
		Info: &openapi3.Info{
			Title:   "Recursive API",
			Version: "1.0.0",
		},
		Paths:      openapi3.NewPaths(),
		Components: &openapi3.Components{}, // Trigger pruning
	}

	op := &openapi3.Operation{
		OperationID: "getUser",
		Responses:   openapi3.NewResponses(),
	}
	op.Responses.Set("200", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Content: openapi3.NewContentWithJSONSchema(userSchema),
		},
	})

	doc.Paths.Set("/user", &openapi3.PathItem{
		Get: op,
	})

	opts := CleanOptions{
		CleanExamples: true,
	}

	// This should not hang or crash
	assert.NotPanics(t, func() {
		CleanAndPruneOpenAPI(doc, opts)
	})
}

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

	opts := CleanOptions{
		KeepTags: []string{"tag1"},
	}

	CleanAndPruneOpenAPI(doc, opts)

	assert.NotNil(t, doc.Paths.Find("/path1"))
	assert.Nil(t, doc.Paths.Find("/path2"))
	assert.NotNil(t, doc.Paths.Find("/path3"))
	assert.Nil(t, doc.Paths.Find("/path4"))

	// Test combination of tags and operation IDs (Intersection)
	doc = &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Paths:   openapi3.NewPaths(),
	}
	op1 = &openapi3.Operation{OperationID: "op1", Tags: []string{"tag1"}}
	op2 = &openapi3.Operation{OperationID: "op2", Tags: []string{"tag2"}}
	op3 = &openapi3.Operation{OperationID: "op3", Tags: []string{"tag1", "tag3"}}
	op4 = &openapi3.Operation{OperationID: "op4", Tags: []string{"tag4"}}

	doc.Paths.Set("/path1", &openapi3.PathItem{Get: op1})
	doc.Paths.Set("/path2", &openapi3.PathItem{Get: op2})
	doc.Paths.Set("/path3", &openapi3.PathItem{Get: op3})
	doc.Paths.Set("/path4", &openapi3.PathItem{Get: op4})

	opts = CleanOptions{
		KeepTags:         []string{"tag1"},
		KeepOperationIDs: []string{"op3"},
	}

	CleanAndPruneOpenAPI(doc, opts)

	assert.Nil(t, doc.Paths.Find("/path1"))    // Has tag1 but not ID op3
	assert.Nil(t, doc.Paths.Find("/path2"))    // Has neither
	assert.NotNil(t, doc.Paths.Find("/path3")) // Has both tag1 and ID op3
	assert.Nil(t, doc.Paths.Find("/path4"))    // Has neither
}
