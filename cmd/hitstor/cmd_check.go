package main

import (
	"fmt"
	"os"

	"github.com/hooperits/hitstor/internal/checker"
	"github.com/hooperits/hitstor/internal/ui"
	"github.com/spf13/cobra"
)

var readDataFlag bool

func newCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Verify repository integrity",
		RunE:  runCheck,
	}

	cmd.Flags().BoolVar(&readDataFlag, "read-data", false, "read and verify all pack data (slower but thorough)")

	return cmd
}

func runCheck(cmd *cobra.Command, args []string) error {
	repo, be, err := openRepository()
	if err != nil {
		return err
	}
	defer be.Close()

	if err := lockRepository(repo); err != nil {
		return fmt.Errorf("lock repository: %w", err)
	}
	defer repo.Unlock()

	c := checker.New(repo)
	color := ui.NewColorWriter(os.Stdout, jsonFlag || quietFlag)

	progress := func(step string) {
		if !quietFlag {
			fmt.Fprintf(os.Stdout, "%s\n", color.Cyan(step))
		}
	}

	errs, err := c.Check(readDataFlag, progress)
	if err != nil {
		return fmt.Errorf("check: %w", err)
	}

	if len(errs) == 0 {
		if !quietFlag {
			fmt.Fprintln(os.Stdout, color.Green("no errors found"))
		}
		return nil
	}

	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "%s\n", color.Red("error: "+e.Message))
	}
	return fmt.Errorf("check found %d error(s)", len(errs))
}
