//go:build linux || darwin

package term

import (
	"os"
	"syscall"
	"unsafe"
)

// termios-механика, общая для Linux и macOS; платформенные номера ioctl —
// в term_sys_{linux,darwin}.go.

func ioctlTermios(fd uintptr, req uintptr, t *syscall.Termios) error {
	_, _, errno := syscall.Syscall(sysIoctl, fd, req, uintptr(unsafe.Pointer(t)))
	if errno != 0 {
		return errno
	}
	return nil
}

func isTerminal(fd uintptr) bool {
	var t syscall.Termios
	return ioctlTermios(fd, ioctlGet, &t) == nil
}

type winsize struct{ rows, cols, x, y uint16 }

func size() (int, int, bool) {
	var ws winsize
	_, _, errno := syscall.Syscall(sysIoctl, os.Stdout.Fd(), ioctlWinsz,
		uintptr(unsafe.Pointer(&ws)))
	if errno != 0 || ws.cols == 0 {
		return 0, 0, false
	}
	return int(ws.cols), int(ws.rows), true
}

func raw() (func(), error) {
	fd := os.Stdin.Fd()
	var saved syscall.Termios
	if err := ioctlTermios(fd, ioctlGet, &saved); err != nil {
		return nil, err
	}
	t := saved
	// без эха и построчного буфера; Ctrl-C оставляем сигналом (ISIG цел) —
	// tui ловит его и восстанавливает терминал; Ctrl-S не морозит вывод
	t.Lflag &^= syscall.ECHO | syscall.ICANON
	t.Iflag &^= syscall.IXON | syscall.ICRNL
	// VMIN=0 + VTIME=1: read возвращается максимум через 100 мс, возможно
	// пустым — цикл TUI живёт без горутин-читателей, крадущих stdin
	t.Cc[syscall.VMIN] = 0
	t.Cc[syscall.VTIME] = 1
	if err := ioctlTermios(fd, ioctlSet, &t); err != nil {
		return nil, err
	}
	return func() { _ = ioctlTermios(fd, ioctlSet, &saved) }, nil
}

// readInput: темп задаёт VTIME (≈100 мс), параметр таймаута не нужен.
// syscall.Read напрямую: os.File превратил бы пустой read в io.EOF.
func readInput(p []byte, _ int) (int, error) {
	n, err := syscall.Read(int(os.Stdin.Fd()), p)
	if n < 0 {
		n = 0
	}
	if err == syscall.EINTR {
		err = nil // сигнал (например SIGWINCH) порвал read — это не беда
	}
	return n, err
}
