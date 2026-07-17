//go:build linux

package term

import "syscall"

const (
	sysIoctl   = syscall.SYS_IOCTL
	ioctlGet   = syscall.TCGETS
	ioctlSet   = syscall.TCSETS
	ioctlWinsz = syscall.TIOCGWINSZ
)
