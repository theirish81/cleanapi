// Package cmd implements the CLI commands for the cleanapi tool.
package cmd

import (
	"fmt"
	"os"

	"github.com/theirish81/cleanapi/lib"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	inputFile         string
	outputFile        string
	noExamples        bool
	operationIds      []string
	tags              []string
	only2xx           bool
	noResponseSchemas bool
	removeExtensions  bool
	stripMetadata     bool
)

// cleanCmd represents the 'clean' command which is the primary entry point for the CLI.
var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean an OpenAPI specification",
	Long: `The clean command processes an OpenAPI specification to remove unwanted elements.
It can follow external references, remove extensions, examples, and keep specific operations by ID or Tag.`,
	RunE: runClean,
}

// runClean orchestrates the loading, cleaning, and saving of the OpenAPI specification.
func runClean(cmd *cobra.Command, args []string) error {
	if inputFile == "" {
		return fmt.Errorf("input file is required")
	}

	// 1. Load the specification (resolving external references if necessary).
	doc, err := loadOpenAPI(inputFile)
	if err != nil {
		return fmt.Errorf("loading OpenAPI spec: %w", err)
	}

	// 2. Perform the cleaning/pruning logic.
	lib.CleanAndPruneOpenAPI(doc, lib.CleanOptions{
		CleanExamples:         noExamples,
		KeepOperationIDs:      operationIds,
		KeepTags:              tags,
		RemoveNon2xxErrors:    only2xx,
		RemoveResponseSchemas: noResponseSchemas,
		RemoveExtensions:      removeExtensions,
		StripMetadata:         stripMetadata,
	})

	// 3. Save the result back to disk.
	if err := saveOpenAPI(doc, outputFile); err != nil {
		return fmt.Errorf("saving OpenAPI spec: %w", err)
	}

	fmt.Printf("Successfully cleaned OpenAPI spec and saved to %s\n", outputFile)
	return nil
}

// loadOpenAPI loads an OpenAPI specification from a file path.
// It enables external reference resolution by default.
func loadOpenAPI(path string) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	return loader.LoadFromFile(path)
}

// saveOpenAPI serializes the OpenAPI document to a YAML file.
// It first converts the document to JSON using kin-openapi's internal logic
// to ensure all special fields (like extensions) are correctly handled,
// then unmarshals into a generic object and finally marshals to YAML for the output.
func saveOpenAPI(doc *openapi3.T, path string) error {
	jsonData, err := doc.MarshalJSON()
	if err != nil {
		return err
	}

	var jsonObj interface{}
	if err := yaml.Unmarshal(jsonData, &jsonObj); err != nil {
		return err
	}

	data, err := yaml.Marshal(jsonObj)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func init() {
	rootCmd.AddCommand(cleanCmd)

	flags := cleanCmd.Flags()
	flags.StringVarP(&inputFile, "input", "i", "", "Input OpenAPI spec file (required)")
	flags.StringVarP(&outputFile, "output", "o", "output.yaml", "Output file path")
	flags.StringSliceVarP(&operationIds, "operation", "O", []string{}, "Operation IDs to keep (all others will be removed)")
	flags.StringSliceVarP(&tags, "tag", "T", []string{}, "Tag names to keep (all operations with these tags will be kept)")
	flags.BoolVar(&noExamples, "no-examples", false, "Remove all example and examples fields")
	flags.BoolVar(&only2xx, "only-2xx", false, "Remove all non-2xx responses")
	flags.BoolVar(&noResponseSchemas, "no-response-schemas", false, "Remove all response schemas")
	flags.BoolVar(&removeExtensions, "remove-extensions", false, "Remove all x- extensions")
	flags.BoolVar(&stripMetadata, "strip-metadata", false, "Removes all root level metadata")
}
