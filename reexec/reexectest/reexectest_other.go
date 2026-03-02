//go:build !linux

package reexectest

import "os/exec"

func setPdeathsig(*exec.Cmd) {}
