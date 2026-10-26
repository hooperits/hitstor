//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hooperits/hitstor/internal/winsvc"
	"github.com/spf13/cobra"
)

// defaultServiceName is the Windows Service name used by the server installer.
const defaultServiceName = "HitStor"

var (
	svcNameFlag    string
	svcDisplayFlag string
)

func init() {
	svcCmd := &cobra.Command{
		Use:   "service",
		Short: "Manage the Windows Service",
		Long: "Install, uninstall, start, or stop the HitStor server (hitstor serve) as a Windows Service.\n\n" +
			"Pass --config so the service uses a fixed config file, e.g.:\n" +
			`  hitstor service install --config C:\ProgramData\HitStor\config.yaml`,
	}

	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install the server as a Windows Service",
		RunE:  runServiceInstall,
	}
	installCmd.Flags().StringVar(&svcNameFlag, "name", defaultServiceName, "service name")
	installCmd.Flags().StringVar(&svcDisplayFlag, "display", "HitStor Server", "display name in Services console")

	uninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the Windows Service",
		RunE:  runServiceUninstall,
	}
	uninstallCmd.Flags().StringVar(&svcNameFlag, "name", defaultServiceName, "service name")

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Windows Service",
		RunE:  runServiceStart,
	}
	startCmd.Flags().StringVar(&svcNameFlag, "name", defaultServiceName, "service name")

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Windows Service",
		RunE:  runServiceStop,
	}
	stopCmd.Flags().StringVar(&svcNameFlag, "name", defaultServiceName, "service name")

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

	// The service runs `hitstor serve`; without --config the service account
	// (LocalSystem) would look for its own per-user config.
	svcArgs := []string{"serve"}
	if configFlag != "" {
		cfgPath, err := filepath.Abs(configFlag)
		if err != nil {
			return fmt.Errorf("resolve config path: %w", err)
		}
		svcArgs = append(svcArgs, "--config", cfgPath)
	}

	if err := winsvc.Install(svcNameFlag, svcDisplayFlag, "HitStor encrypted backup server and dashboard", exePath, svcArgs); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "Service %q installed.\n", svcNameFlag)
	fmt.Fprintf(os.Stdout, "Start it with: hitstor service start --name %s\n", svcNameFlag)
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
