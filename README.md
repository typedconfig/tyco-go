# Tyco Go Binding

Go implementation of the [Tyco](../tyco-website/v0.1.0.html) configuration language parser.  
This library mirrors the canonical Python parser and is kept in lockstep with the shared
`tyco-test-suite` golden fixtures that live at the workspace root.

## Requirements

- Go 1.21 or newer
- The `tyco-test-suite` directory checked out alongside `tyco-go` (already present in this repo)

## Installation

From the workspace root you can add the module to another project with:

```bash
go get github.com/typedconfig/tyco-go
```

When working directly inside this repository no additional steps are necessary; `go test` will
build and run against the local module source.

## Quick Start

This package includes a ready-to-use example Tyco file at:

	example.tyco

([View on GitHub](https://github.com/typedconfig/tyco-go/blob/main/example.tyco))

You can load and parse this file using the Go Tyco API. Example usage:

```go
package main

import (
	"fmt"
	tyco "github.com/typedconfig/tyco-go"
)

func main() {
	ctx, err := tyco.Load("example.tyco")
	if err != nil {
		panic(err)
	}
	globals := ctx.Globals
	environment := globals["environment"]
	debug := globals["debug"]
	timeout := globals["timeout"]
	fmt.Printf("env=%v debug=%v timeout=%v\n", environment, debug, timeout)
	// ... access objects, etc ...
}
```

See the [example.tyco](https://github.com/typedconfig/tyco-go/blob/main/example.tyco) file for the full configuration example.

	// Materialise the document as plain Go maps/slices identical to the reference JSON.
	data := ctx.ToJSON()
	fmt.Printf("Project: %s\n", data["project"])
}
```

You can also parse from a string using `tyco.LoadString(content)`, which is convenient for tests
or embedding.

## Development

All changes must continue to pass the canonical suite:

```bash
cd tyco-go
go test ./...
```

The golden test automatically runs every `.tyco` file from `../tyco-test-suite/inputs` and compares
the parser output against the JSON files in `../tyco-test-suite/expected`. This keeps the Go
binding behaviour consistent with the other language ports.

Please run `gofmt -w *.go` (or rely on `go test`, which fails on syntax errors) before sending a
change upstream.
