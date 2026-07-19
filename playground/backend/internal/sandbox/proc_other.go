//go:build !unix

package sandbox

import "os/exec"

// Не-Unix: групп процессов нет — убиваем только сам процесс. Честная
// деградация в духе internal/term самой библиотеки.
func setProcGroup(cmd *exec.Cmd) {}

func killProcGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
