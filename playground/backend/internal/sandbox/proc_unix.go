//go:build unix

package sandbox

import (
	"os/exec"
	"syscall"
)

// setProcGroup — своя группа процессов: убивая по таймауту, убьём и детей
// (пользовательский код мог наплодить exec'ов).
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	// отрицательный pid — сигнал всей группе
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
