# CleanAPI

CleanAPI is a high-performance Go library and CLI utility designed to simplify and prune OpenAPI 3.0 specifications. It allows you to transform bloated, auto-generated, or internal specs into clean, consumer-ready documentation.

## Why

OpenAPI specifications—especially those generated from code—often contain a significant amount of "noise" that can overwhelm consumers, inflate file sizes, and leak internal details. Sometimes the spec file is simply too bloated and complex for the specific purpose you want to use it for (e.g., generating client SDKs, documentation, or mock servers). Common issues include:
-   **Leaked Extensions**: Internal `x-` metadata used by specific tools (like code generators or gateways) that aren't relevant to external users.
-   **Recursive Bloat**: Huge schemas with deep recursion or unnecessary examples.
-   **Unused Components**: Orphaned schemas, responses, or parameters that remain in the `components` section after paths are removed.
-   **Non-Production Responses**: Error responses (4xx, 5xx) or internal endpoints that you may wish to hide from a public-facing spec.

CleanAPI provides a surgical way to strip this noise while maintaining a valid, functional specification.

## What It Does

CleanAPI performs a multi-stage cleaning and pruning process:

1.  **Metadata Stripping: Removes licenses, contact info, and external documentation if requested.
2.  **Surgical Filtering**: Keep only the operations you want. It automatically prunes empty paths.
3.  **Content Cleaning**: Strips examples, strips non-2xx responses, and can even remove response schemas entirely (leaving only the status code).
4.  **Extension Management**: Optionally strips all `x-` extensions document-wide.
5.  **Smart GC (Garbage Collection)**: A recursive "mark-and-sweep" pruning engine identifies and removes all components (schemas, headers, responses, etc.) that are no longer reachable from the remaining operations.
6.  **Recursive Safety**: Safely handles circular references in schemas and component dependencies without stack overflows.
7.  **External Reference Resolution**: Correctly follows and resolves `$ref` pointers to external files.

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
