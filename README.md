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

	data := ctx.ToObject()
	fmt.Printf("timezone=%v\n", data["timezone"])

	if apps, ok := data["Application"].([]any); ok && len(apps) > 0 {
		if primary, ok := apps[0].(map[string]any); ok {
			fmt.Printf("primary service -> %s (%s)\n", primary["service"], primary["command"])
		}
	}

	if hosts, ok := data["Host"].([]any); ok && len(hosts) > 1 {
		if backup, ok := hosts[1].(map[string]any); ok {
			fmt.Printf("host %s cores=%v\n", backup["hostname"], backup["cores"])
		}
	}
}
```

Use `tyco.LoadString(content)` in tests to parse from memory; it returns the same `*TycoContext`
so you can call `ToObject()` to inspect globals and struct instances.

### Example Tyco File

```
tyco/example.tyco
```

```tyco
str timezone: UTC  # this is a global config setting

Application:       # schema defined first, followed by instance creation
  str service:
  str profile:
  str command: start_app {service}.{profile} -p {port.number}
  Host host:
  Port port: Port(http_web)  # reference to Port instance defined below
  - service: webserver, profile: primary, host: Host(prod-01-us)
  - service: webserver, profile: backup,  host: Host(prod-02-us)
  - service: database,  profile: mysql,   host: Host(prod-02-us), port: Port(http_mysql)

Host:
 *str hostname:  # star character (*) used as reference primary key
  int cores:
  bool hyperthreaded: true
  str os: Debian
  - prod-01-us, cores: 64, hyperthreaded: false
  - prod-02-us, cores: 32, os: Fedora

Port:
 *str name:
  int number:
  - http_web,   80  # can skip field keys when obvious
  - http_mysql, 3306
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
