package agent

import (
	"os/exec"
)

func runCommand(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}
