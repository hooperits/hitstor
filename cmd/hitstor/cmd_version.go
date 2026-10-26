package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"github.com/hooperits/hitstor/internal/update"
	"github.com/spf13/cobra"
)

var checkFlag bool

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version and build information",
		Args:  cobra.NoArgs,
		RunE:  runVersion,
	}
	cmd.Flags().BoolVar(&checkFlag, "check", false, "check for available updates")
	return cmd
}

func runVersion(cmd *cobra.Command, args []string) error {
	if checkFlag {
		updateURL := loadedConfig.UpdateURL
		result, err := update.Check(Version, updateURL)
		if err != nil {
			return fmt.Errorf("version: check update: %w", err)
		}
		fmt.Fprintln(os.Stdout, update.FormatResult(result))
		return nil
	}

	if jsonFlag {
		data := map[string]string{
			"version": Version,
			"commit":  Commit,
			"date":    Date,
			"os":      runtime.GOOS,
			"arch":    runtime.GOARCH,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(data)
	}

	fmt.Fprintf(os.Stdout, "hitstor v%s (commit %s, built %s, %s/%s)\n",
		Version, Commit, Date, runtime.GOOS, runtime.GOARCH)
	return nil
}
