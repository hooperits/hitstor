//go:build windows

// Package winsvc provides Windows Service management for the HitStor server and agent.
package winsvc

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

// IsService reports whether the process is running as a Windows Service.
func IsService() bool {
	ok, _ := svc.IsWindowsService()
	return ok
}

// Install registers exePath (started with args) as an automatic-start Windows Service.
func Install(name, displayName, description, exePath string, args []string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.CreateService(name, exePath, mgr.Config{
		DisplayName: displayName,
		StartType:   mgr.StartAutomatic,
		Description: description,
	}, args...)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer s.Close()

	// Create an event log source so the service can write to Event Viewer.
	if err := eventlog.InstallAsEventCreate(name, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		// Non-fatal: service works without event log.
		s.Delete()
		return fmt.Errorf("install event log: %w", err)
	}

	return nil
}

// Uninstall removes the named Windows Service.
func Uninstall(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("open service %q: %w", name, err)
	}
	defer s.Close()

	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}

	_ = eventlog.Remove(name)
	return nil
}

// Start starts the named Windows Service.
func Start(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("open service %q: %w", name, err)
	}
	defer s.Close()

	return s.Start()
}

// Stop sends a stop signal to the named Windows Service and waits up to 30 seconds.
func Stop(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("open service %q: %w", name, err)
	}
	defer s.Close()

	status, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("send stop: %w", err)
	}

	timeout := time.Now().Add(30 * time.Second)
	for status.State != svc.Stopped {
		if time.Now().After(timeout) {
			return fmt.Errorf("timeout waiting for service to stop")
		}
		time.Sleep(500 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("query service status: %w", err)
		}
	}
	return nil
}

// handler implements the svc.Handler interface for running as a Windows Service.
type handler struct {
	run func(ctx context.Context) error
}

// Execute is called by the Windows service control manager.
func (h *handler) Execute(_ []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- h.run(ctx)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: accepted}

	for {
		select {
		case err := <-errCh:
			changes <- svc.Status{State: svc.StopPending}
			if err != nil {
				return true, 1
			}
			return false, 0

		case c := <-r:
			switch c.Cmd {
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				// Wait for the run function to finish.
				<-errCh
				return false, 0

			case svc.Interrogate:
				changes <- c.CurrentStatus
			}
		}
	}
}

// Run executes the given function as a Windows Service. The function receives
// a context that is cancelled when the service receives a stop/shutdown signal.
func Run(name string, run func(ctx context.Context) error) error {
	return svc.Run(name, &handler{run: run})
}
