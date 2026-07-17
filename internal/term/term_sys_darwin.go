//go:build darwin

package term

import "syscall"

const (
	sysIoctl   = syscall.SYS_IOCTL
	ioctlGet   = syscall.TIOCGETA
	ioctlSet   = syscall.TIOCSETA
	ioctlWinsz = syscall.TIOCGWINSZ
)
