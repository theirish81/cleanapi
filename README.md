# CleanAPI

CleanAPI is a Go library and CLI utility designed to surgically prune and optimize OpenAPI 3.0 specifications. It allows you to transform bloated, auto-generated, or internal specs into lean, purpose-built definitions optimized for machine consumers like client SDK generators, mock servers, and internal tooling.

## Why

OpenAPI specifications—especially those generated from code—often contain significant technical debt and "noise" that can break generators, inflate file sizes, and leak internal implementation details. A spec intended for a public SDK should not contain internal extensions, unused schemas, or non-production error responses.

Common issues that CleanAPI solves:
-   **Generator Noise**: Internal `x-` metadata used by specific tools (like gateways) that can confuse or bloat client SDK generators.
-   **Schema Bloat**: Huge schemas with deep recursion or unnecessary examples that increase bundle size in generated clients.
-   **Unused Components**: Orphaned schemas, responses, or parameters that remain in the `components` section, leading to "dead code" in generated models.
-   **Internal Leakage**: Non-production responses (4xx, 5xx) or internal endpoints that shouldn't be exposed to specific consumers.

CleanAPI provides a surgical way to strip this noise, ensuring your spec is as lean as possible for its specific target use case.

## What It Does

CleanAPI performs a multi-stage technical pruning process:

1.  **Surgical Filtering**: Keep only the operations you actually need for a specific consumer.
2.  **Smart Garbage Collection**: A recursive "mark-and-sweep" engine identifies and removes all top-level components (schemas, headers, responses) that are no longer reachable from the remaining operations.
3.  **Content Stripping**: Removes examples, non-2xx responses, or even entire response schemas to minimize the footprint of generated code.
4.  **Extension Management**: Optionally strips all `x-` extensions document-wide to avoid generator-specific conflicts.
5.  **Recursive Safety**: Safely handles circular references and complex component dependencies.
6.  **Metadata Pruning**: Removes licenses, contact info, and external documentation to keep the spec focused on the interface definition.
7.  **External Reference Resolution**: Correctly follows and resolves `$ref` pointers to external files during the pruning process.

## CLI

The CLI is a standalone utility for processing files.

### Installation
```bash
go build -o cleanapi
```

### Usage
```bash
cleanapi clean [flags]
```

### Flags
-   `-i, --input <file>`: (Required) The input OpenAPI specification (YAML or JSON).
-   `-o, --output <file>`: Output path (default: `output.yaml`).
-   `-O, --operation <id1,id2>`: List of Operation IDs to keep. All others will be removed.
-   `--no-examples`: Remove all `example` and `examples` fields.
-   `--only-2xx`: Remove all non-2xx responses (and the `default` response).
-   `--no-response-schemas`: Remove the `content` field from all responses.
-   `--remove-extensions`: Remove all custom `x-` extensions.
-   `--strip-metadata`: Removes all the unnecessary info metadata

### Example
```bash
cleanapi clean -i internal-api.yaml -o public-api.yaml --only-2xx --no-examples --remove-extensions
```

## Library

CleanAPI can be used as a Go library within your own tools or pipelines.

### Basic Usage
```go
import (
    "cleanapi/lib"
    "github.com/getkin/kin-openapi/openapi3"
)

func main() {
    // 1. Load your doc (using kin-openapi)
    loader := openapi3.NewLoader()
    doc, _ := loader.LoadFromFile("openapi.yaml")

    // 2. Define your cleaning options
    opts := lib.CleanOptions{
        CleanExamples:      true,
        RemoveExtensions:   true,
        RemoveNon2xxErrors: true,
    }

    // 3. Clean and Prune
    lib.CleanAndPruneOpenAPI(doc, opts)

    // doc is now modified in-place and ready for marshaling
}
```

### CleanOptions
-   `CleanExamples`: Strips example data.
-   `KeepOperationIDs`: Filters the spec to a specific subset of operations.
-   `RemoveNon2xxErrors`: Strips error responses.
-   `RemoveResponseSchemas`: Strips response body definitions.
-   `RemoveExtensions`: Strips all `x-` extensions.
-   `StripMetadata`: Removes all the unnecessary info metadata
