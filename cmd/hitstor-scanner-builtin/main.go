// hitstor-scanner-builtin is a reference filesystem scanner plugin that wraps
// the built-in default scanner. It demonstrates the scanner plugin interface
// and serves as a template for custom scanner plugins.
package main

import (
	"github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/scanner"
)

func main() {
	plugin.ServeFilesystemScanner(scanner.NewDefault())
}
