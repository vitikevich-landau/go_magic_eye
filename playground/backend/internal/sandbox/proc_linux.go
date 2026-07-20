//go:build linux

package sandbox

import (
	"syscall"
	"unsafe"
)

// applyMemLimit — жёсткий потолок адресного пространства чужого процесса
// (prlimit64 + RLIMIT_AS). GOMEMLIMIT — лишь цель GC, снипетт может съесть
// сильно больше; rlimit — настоящий заслон: mmap сверх потолка откажет, и
// Go-рантайм честно упадёт с out of memory. Обёртка unshare исполняется
// без -f (exec на месте, pid тот же) — лимит достаётся самому снипетту.
//
// Порог выбирается с запасом: Go-рантайм на старте резервирует сотни МиБ
// виртуального пространства (замер: снипетту Ока нужно 512–768 МиБ AS).
func applyMemLimit(pid int, bytes int64) error {
	if bytes <= 0 {
		return nil // потолок выключен явно
	}
	lim := syscall.Rlimit{Cur: uint64(bytes), Max: uint64(bytes)}
	_, _, errno := syscall.Syscall6(syscall.SYS_PRLIMIT64,
		uintptr(pid), uintptr(syscall.RLIMIT_AS),
		uintptr(unsafe.Pointer(&lim)), 0, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
