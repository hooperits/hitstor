package main

import (
	"fmt"
	"os"

	"github.com/hooperits/hitstor/internal/agent"
	"github.com/spf13/cobra"
)

func newShowIDCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show-id",
		Short: "Display the agent token for dashboard registration",
		Long: `Print the agent token (64-character hex string).
Use this token when adding the agent from the HitStor dashboard.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := agent.LoadToken(agent.DefaultTokenPath)
			if err != nil {
				return fmt.Errorf("could not read agent token: %w\nIs the agent installed?", err)
			}
			fmt.Fprintln(os.Stdout, token)
			return nil
		},
	}
}
