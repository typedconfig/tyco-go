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

Every binding ships the same canonical sample configuration under `tyco/example.tyco`
([view on GitHub](https://github.com/typedconfig/tyco-go/blob/main/tyco/example.tyco)).
Load it to explore globals, structs, and references exactly like in the Python README:

```go
package main

import (
	"fmt"
	"log"
	tyco "github.com/typedconfig/tyco-go"
)

func main() {
	ctx, err := tyco.Load("tyco/example.tyco")
	if err != nil {
		log.Fatal(err)
	}

	data := ctx.ToJSON()
	fmt.Printf("env=%v debug=%v timeout=%v\n", data["environment"], data["debug"], data["timeout"])

	if databases, ok := data["Database"].([]any); ok && len(databases) > 0 {
		if primary, ok := databases[0].(map[string]any); ok {
			fmt.Printf("primary database -> %s:%v\n", primary["host"], primary["port"])
		}
	}
}
```

Use `tyco.LoadString(content)` in tests to parse from memory; it returns the same `*TycoContext`
so you can call `ToJSON()` to inspect globals and struct instances.

### Example Tyco File

```
tyco/example.tyco
```

```tyco
# Global configuration with type annotations
str environment: production
bool debug: false
int timeout: 30

# Database configuration struct
Database:
 *str name:           # Primary key field (*)
  str host:
  int port:
  str connection_string:
  # Instances
  - primary, localhost,    5432, "postgresql://localhost:5432/myapp"
  - replica, replica-host, 5432, "postgresql://replica-host:5432/myapp"

# Server configuration struct  
Server:
 *str name:           # Primary key for referencing
  int port:
  str host:
  ?str description:   # Nullable field (?) - can be null
  # Server instances
  - web1,    8080, web1.example.com,    description: "Primary web server"
  - api1,    3000, api1.example.com,    description: null
  - worker1, 9000, worker1.example.com, description: "Worker number 1"

# Feature flags array
str[] features: [auth, analytics, caching]
```

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
