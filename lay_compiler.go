package main

// Thin wrapper: the lint sister compiler now lives in the importable
// ./lint package (pure in-memory). This file keeps `go run main.go
// lay_compiler.go` working byte-identically.

import (
	"github.com/Xelckis/floppyURL/lint"
)

// CompileLAY delegates to lint.Compile.
func CompileLAY(src []byte) ([]byte, error) {
	return lint.Compile(src)
}
