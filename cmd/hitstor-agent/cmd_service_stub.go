//go:build !windows

package main

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

var svcNameFlag string

func init() {
	svcCmd := &cobra.Command{
		Use:   "service",
		Short: "Manage the HitStor agent as a system service (systemd)",
		Long:  "Control the hitstor-agent systemd service. Installation is typically handled by the package manager.",
	}

	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Enable and start the service (may require package install first)",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := svcNameFlag
			if name == "" {
				name = "hitstor-agent"
			}
			// Try to enable and start
			if err := exec.Command("systemctl", "enable", "--now", name).Run(); err != nil {
				return fmt.Errorf("enable service: %w (is the unit installed? see packaging/systemd)", err)
			}
			fmt.Printf("Service %q enabled and started.\n", name)
			return nil
		},
	}
	installCmd.Flags().StringVar(&svcNameFlag, "name", "hitstor-agent", "systemd unit name")

	uninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Disable and stop the service",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := svcNameFlag
			if name == "" {
				name = "hitstor-agent"
			}
			_ = exec.Command("systemctl", "stop", name).Run()
			if err := exec.Command("systemctl", "disable", name).Run(); err != nil {
				return fmt.Errorf("disable service: %w", err)
			}
			fmt.Printf("Service %q disabled.\n", name)
			return nil
		},
	}
	uninstallCmd.Flags().StringVar(&svcNameFlag, "name", "hitstor-agent", "systemd unit name")

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the service",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := svcNameFlag
			if name == "" {
				name = "hitstor-agent"
			}
			if err := exec.Command("systemctl", "start", name).Run(); err != nil {
				return fmt.Errorf("start service: %w", err)
			}
			fmt.Printf("Service %q started.\n", name)
			return nil
		},
	}
	startCmd.Flags().StringVar(&svcNameFlag, "name", "hitstor-agent", "systemd unit name")

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the service",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := svcNameFlag
			if name == "" {
				name = "hitstor-agent"
			}
			if err := exec.Command("systemctl", "stop", name).Run(); err != nil {
				return fmt.Errorf("stop service: %w", err)
			}
			fmt.Printf("Service %q stopped.\n", name)
			return nil
		},
	}
	stopCmd.Flags().StringVar(&svcNameFlag, "name", "hitstor-agent", "systemd unit name")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show service status",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := svcNameFlag
			if name == "" {
				name = "hitstor-agent"
			}
			out, _ := exec.Command("systemctl", "status", name).CombinedOutput()
			fmt.Print(string(out))
			return nil
		},
	}
	statusCmd.Flags().StringVar(&svcNameFlag, "name", "hitstor-agent", "systemd unit name")

	enableCmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable the service to start on boot",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := svcNameFlag
			if name == "" {
				name = "hitstor-agent"
			}
			if err := exec.Command("systemctl", "enable", name).Run(); err != nil {
				return fmt.Errorf("enable service: %w", err)
			}
			fmt.Printf("Service %q enabled.\n", name)
			return nil
		},
	}
	enableCmd.Flags().StringVar(&svcNameFlag, "name", "hitstor-agent", "systemd unit name")

	disableCmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable the service from starting on boot",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := svcNameFlag
			if name == "" {
				name = "hitstor-agent"
			}
			if err := exec.Command("systemctl", "disable", name).Run(); err != nil {
				return fmt.Errorf("disable service: %w", err)
			}
			fmt.Printf("Service %q disabled.\n", name)
			return nil
		},
	}
	disableCmd.Flags().StringVar(&svcNameFlag, "name", "hitstor-agent", "systemd unit name")

	restartCmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the service",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := svcNameFlag
			if name == "" {
				name = "hitstor-agent"
			}
			if err := exec.Command("systemctl", "restart", name).Run(); err != nil {
				return fmt.Errorf("restart service: %w", err)
			}
			fmt.Printf("Service %q restarted.\n", name)
			return nil
		},
	}
	restartCmd.Flags().StringVar(&svcNameFlag, "name", "hitstor-agent", "systemd unit name")

	svcCmd.AddCommand(installCmd, uninstallCmd, startCmd, stopCmd, statusCmd, enableCmd, disableCmd, restartCmd)
	rootCmd.AddCommand(svcCmd)
}
