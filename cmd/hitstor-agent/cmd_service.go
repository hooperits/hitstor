//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hooperits/hitstor/internal/winsvc"
	"github.com/spf13/cobra"
)

var (
	svcNameFlag    string
	svcDisplayFlag string
)

func init() {
	svcCmd := &cobra.Command{
		Use:   "service",
		Short: "Manage the Windows Service",
		Long:  "Install, uninstall, start, or stop the HitStor agent as a Windows Service.",
	}

	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install the agent as a Windows Service",
		RunE:  runServiceInstall,
	}
	installCmd.Flags().StringVar(&svcNameFlag, "name", "HitStorAgent", "service name")
	installCmd.Flags().StringVar(&svcDisplayFlag, "display", "HitStor Backup Agent", "display name in Services console")

	uninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the Windows Service",
		RunE:  runServiceUninstall,
	}
	uninstallCmd.Flags().StringVar(&svcNameFlag, "name", "HitStorAgent", "service name")

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Windows Service",
		RunE:  runServiceStart,
	}
	startCmd.Flags().StringVar(&svcNameFlag, "name", "HitStorAgent", "service name")

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Windows Service",
		RunE:  runServiceStop,
	}
	stopCmd.Flags().StringVar(&svcNameFlag, "name", "HitStorAgent", "service name")

	svcCmd.AddCommand(installCmd, uninstallCmd, startCmd, stopCmd)
	rootCmd.AddCommand(svcCmd)
}

func runServiceInstall(cmd *cobra.Command, args []string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("resolve absolute path: %w", err)
	}

	// Build service arguments: backup command with the same flags.
	var svcArgs []string
	if serverFlag != "" {
		svcArgs = append(svcArgs, "--server", serverFlag)
	}
	if apiKeyFlag != "" {
		svcArgs = append(svcArgs, "--api-key", apiKeyFlag)
	}
	if caCertFlag != "" {
		svcArgs = append(svcArgs, "--ca-cert", caCertFlag)
	}
	if configFlag != "" {
		svcArgs = append(svcArgs, "--config", configFlag)
	}

	if err := winsvc.Install(svcNameFlag, svcDisplayFlag, exePath, svcArgs); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "Service %q installed.\n", svcNameFlag)
	fmt.Fprintf(os.Stdout, "Start it with: hitstor-agent service start --name %s\n", svcNameFlag)
	return nil
}

func runServiceUninstall(cmd *cobra.Command, args []string) error {
	if err := winsvc.Uninstall(svcNameFlag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Service %q removed.\n", svcNameFlag)
	return nil
}

func runServiceStart(cmd *cobra.Command, args []string) error {
	if err := winsvc.Start(svcNameFlag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Service %q started.\n", svcNameFlag)
	return nil
}

func runServiceStop(cmd *cobra.Command, args []string) error {
	if err := winsvc.Stop(svcNameFlag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Service %q stopped.\n", svcNameFlag)
	return nil
}
