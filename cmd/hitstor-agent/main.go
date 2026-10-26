package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hooperits/hitstor/internal/ui"
)

// Build metadata injected via -ldflags at build time.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func main() {
	// On Windows, detect if running as a service and enter service mode.
	if runAsService() {
		return
	}

	if err := rootCmd.Execute(); err != nil {
		color := ui.NewColorWriter(os.Stderr, jsonFlag || quietFlag)
		hint := ui.FormatError(err)

		if jsonFlag {
			data := map[string]string{"error": hint.Message}
			if hint.Hint != "" {
				data["hint"] = hint.Hint
			}
			enc := json.NewEncoder(os.Stderr)
			enc.SetEscapeHTML(false)
			enc.Encode(data)
		} else {
			fmt.Fprintln(os.Stderr, color.Red("Error: "+hint.Message))
			if hint.Hint != "" {
				fmt.Fprintln(os.Stderr, color.Yellow("Hint:  "+hint.Hint))
			}
		}
		os.Exit(1)
	}
}
