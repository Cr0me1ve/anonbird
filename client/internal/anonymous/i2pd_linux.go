//go:build linux

package anonymous

import (
	"os/exec"
	"syscall"
)

func setI2PCommandCredential(cmd *exec.Cmd, uid, gid int) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		},
	}
}
