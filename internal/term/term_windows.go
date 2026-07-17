//go:build windows

package term

import (
	"os"
	"syscall"
	"unsafe"
)

// Консоль Windows: режимы через kernel32, дальше — обычные ANSI/VT
// последовательности (Windows 10+). Внешних зависимостей нет — только syscall.

var (
	kernel32                   = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode         = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode         = kernel32.NewProc("SetConsoleMode")
	procGetConsoleScreenBuffer = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procWaitForSingleObject    = kernel32.NewProc("WaitForSingleObject")
)

const waitObject0 = 0

const (
	enableEchoInput      = 0x0004
	enableLineInput      = 0x0002
	enableProcessedInput = 0x0001
	enableMouseInput     = 0x0010
	enableVTInput        = 0x0200 // стрелки приходят ESC-последовательностями
	enableProcessedOut   = 0x0001
	enableVTProcessing   = 0x0004 // консоль понимает ANSI-цвета и alt-screen
)

func getMode(h uintptr) (uint32, bool) {
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)))
	return mode, r != 0
}

func setMode(h uintptr, mode uint32) bool {
	r, _, _ := procSetConsoleMode.Call(h, uintptr(mode))
	return r != 0
}

func isTerminal(fd uintptr) bool {
	_, ok := getMode(fd)
	return ok
}

type coord struct{ x, y int16 }
type smallRect struct{ left, top, right, bottom int16 }
type consoleInfo struct {
	size      coord
	cursor    coord
	attrs     uint16
	window    smallRect
	maxWinSize coord
}

func size() (int, int, bool) {
	var ci consoleInfo
	r, _, _ := procGetConsoleScreenBuffer.Call(os.Stdout.Fd(), uintptr(unsafe.Pointer(&ci)))
	if r == 0 {
		return 0, 0, false
	}
	return int(ci.window.right-ci.window.left) + 1, int(ci.window.bottom-ci.window.top) + 1, true
}

func raw() (func(), error) {
	in, out := os.Stdin.Fd(), os.Stdout.Fd()
	inMode, okIn := getMode(in)
	outMode, okOut := getMode(out)
	if !okIn || !okOut {
		return nil, syscall.ENOTTY
	}
	newIn := inMode &^ uint32(enableEchoInput|enableLineInput|enableProcessedInput|enableMouseInput)
	newIn |= enableVTInput
	newOut := outMode | enableProcessedOut | enableVTProcessing
	if !setMode(in, newIn) || !setMode(out, newOut) {
		setMode(in, inMode)
		setMode(out, outMode)
		return nil, syscall.EINVAL
	}
	return func() {
		setMode(in, inMode)
		setMode(out, outMode)
	}, nil
}

// readInput: ждём готовности консоли не дольше timeoutMS, затем читаем.
// В VT-режиме клавиши приходят теми же ESC-последовательностями, что на Unix.
func readInput(p []byte, timeoutMS int) (int, error) {
	h := os.Stdin.Fd()
	r, _, _ := procWaitForSingleObject.Call(h, uintptr(timeoutMS))
	if r != waitObject0 {
		return 0, nil // таймаут или ошибка ожидания — считаем тишиной
	}
	return syscall.Read(syscall.Handle(h), p)
}
