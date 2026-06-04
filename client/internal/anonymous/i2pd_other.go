//go:build !linux

package anonymous

import "os/exec"

func setI2PCommandCredential(_ *exec.Cmd, _, _ int) {}
