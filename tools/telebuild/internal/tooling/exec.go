package tooling

import (
	"os/exec"
)

func RunCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}
