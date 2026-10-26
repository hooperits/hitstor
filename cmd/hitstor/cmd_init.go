package main

import (
	"fmt"
	"os"

	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/ui"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a new backup repository",
		RunE:  runInit,
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	password, err := getPassword()
	if err != nil {
		return err
	}

	progress := ui.NewProgress(os.Stdout, jsonFlag, quietFlag)

	be, err := openBackend()
	if err != nil {
		return fmt.Errorf("create backend: %w", err)
	}
	defer be.Close()

	repo, err := repository.Init(be, password)
	if err != nil {
		return fmt.Errorf("init repository: %w", err)
	}

	progress.InitComplete(repo.Cfg().ID, repoFlag)
	return nil
}
