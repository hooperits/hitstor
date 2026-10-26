package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hooperits/hitstor/internal/agent"
	"github.com/hooperits/hitstor/internal/ui"
	"github.com/spf13/cobra"
)

func newPingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Check server connectivity",
		Long:  "Ping the HitStor server to verify it is reachable and display its version and repository ID.",
		RunE:  runPing,
	}
}

func runPing(cmd *cobra.Command, args []string) error {
	if err := requireServer(); err != nil {
		return err
	}

	client, err := agent.NewClient(agent.ClientOptions{
		ServerAddr: serverFlag,
		CACertPath: caCertFlag,
		PinPath:    agent.DefaultServerPinPath,
	})
	if err != nil {
		return err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.Ping(ctx)
	if err != nil {
		return err
	}

	color := ui.NewColorWriter(os.Stdout, jsonFlag || quietFlag)
	fmt.Fprintf(os.Stdout, "%s %s\n", color.Green("Server:"), serverFlag)
	fmt.Fprintf(os.Stdout, "%s %s\n", color.Green("Version:"), resp.Version)
	fmt.Fprintf(os.Stdout, "%s %s\n", color.Green("Repo ID:"), resp.RepoId)
	return nil
}
