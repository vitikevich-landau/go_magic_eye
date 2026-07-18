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
	procPeekConsoleInput       = kernel32.NewProc("PeekConsoleInputW")
	procReadConsoleInput       = kernel32.NewProc("ReadConsoleInputW")
	procGetConsoleCP           = kernel32.NewProc("GetConsoleCP")
	procSetConsoleCP           = kernel32.NewProc("SetConsoleCP")
)

const (
	waitObject0 = 0x000
	waitTimeout = 0x102
	cpUTF8      = 65001
	keyEvent    = 0x0001 // INPUT_RECORD.EventType: клавиатура
)

// inputRecord — INPUT_RECORD из wincon.h: EventType + 2 байта выравнивания +
// союз событий (16 байт, читаем из него только bKeyDown по смещению 0).
// Размер 20 байт одинаков на 386 и amd64.
type inputRecord struct {
	eventType uint16
	_         uint16
	data      [16]byte
}

// keyProducesBytes — даст ли эта запись очереди байты при ReadFile.
// Интересны только НАЖАТИЯ (bKeyDown), и не любых клавиш: key-down чистого
// модификатора (Shift, Ctrl, Alt, Win, *Lock) байтов не порождает — ReadFile
// на нём заблокировался бы так же, как на фокус-событии.
// Раскладка KEY_EVENT_RECORD в data: bKeyDown [0:4], wRepeatCount [4:6],
// wVirtualKeyCode [6:8], … (wincon.h).
func (r *inputRecord) keyProducesBytes() bool {
	if r.eventType != keyEvent || *(*int32)(unsafe.Pointer(&r.data[0])) == 0 {
		return false
	}
	vk := *(*uint16)(unsafe.Pointer(&r.data[6]))
	switch vk {
	case 0x10, 0x11, 0x12, // VK_SHIFT, VK_CONTROL, VK_MENU (Alt)
		0x14,       // VK_CAPITAL (CapsLock)
		0x5b, 0x5c, // VK_LWIN, VK_RWIN
		0x90, 0x91: // VK_NUMLOCK, VK_SCROLL
		return false
	}
	return true
}

// Флаги консольных режимов (wincon.h). Смысл зеркален termios-флагам Unix:
//
//	вход:  ECHO_INPUT      ~ ECHO    — консоль сама печатает нажатое;
//	       LINE_INPUT      ~ ICANON  — копить строку до Enter;
//	       PROCESSED_INPUT ~ ISIG    — Ctrl-C превращается в событие, а не
//	                                   байт; мы его ВЫКЛЮЧАЕМ и получаем
//	                                   0x03 обычным байтом (декодер отдаёт
//	                                   KCtrlC — единый путь выхода);
//	       VIRTUAL_TERMINAL_INPUT    — стрелки/F-клавиши приходят теми же
//	                                   ESC-последовательностями, что на
//	                                   Unix: один декодер на все ОС.
//	выход: VIRTUAL_TERMINAL_PROCESSING — консоль исполняет ANSI: цвета,
//	                                   позиционирование, альтернативный
//	                                   экран (Windows 10+).
const (
	enableEchoInput      = 0x0004
	enableLineInput      = 0x0002
	enableProcessedInput = 0x0001
	enableMouseInput     = 0x0010
	enableVTInput        = 0x0200
	enableProcessedOut   = 0x0001
	enableVTProcessing   = 0x0004
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

// enableColor: научить консоль за fd ANSI-цветам ДО первой печати (иначе
// Inspect в старой conhost вывел бы «←[38;5;…m» буквально). Именно за fd:
// у stdout и stderr свои хэндлы со своими режимами. VT остаётся включённым —
// это штатное состояние современных консолей.
func enableColor(fd uintptr) bool {
	mode, ok := getMode(fd)
	if !ok {
		return false
	}
	return setMode(fd, mode|enableProcessedOut|enableVTProcessing)
}

type coord struct{ x, y int16 }
type smallRect struct{ left, top, right, bottom int16 }
type consoleInfo struct {
	size       coord
	cursor     coord
	attrs      uint16
	window     smallRect
	maxWinSize coord
}

func size(fd uintptr) (int, int, bool) {
	var ci consoleInfo
	r, _, _ := procGetConsoleScreenBuffer.Call(fd, uintptr(unsafe.Pointer(&ci)))
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
	// кодовая страница ввода → UTF-8: иначе кириллица в поиске приезжает
	// в OEM-кодировке консоли и декодер видит мусор
	savedCP, _, _ := procGetConsoleCP.Call()
	procSetConsoleCP.Call(cpUTF8)
	return func() {
		procSetConsoleCP.Call(savedCP)
		setMode(in, inMode)
		setMode(out, outMode)
	}, nil
}

// readInput: ждём готовности консоли не дольше timeoutMS, затем читаем.
// В VT-режиме клавиши приходят теми же ESC-последовательностями, что на Unix.
//
// Ловушка, ради которой всё усложнено: хендл консоли «сигналится» ЛЮБЫМИ
// событиями — фокусом окна, отпусканием клавиш, мышью. ReadFile же вернёт
// байты только от НАЖАТИЙ, а на прочем заблокируется намертво — и таймаут
// цикла перестал бы работать. Поэтому перед чтением подглядываем очередь
// (PeekConsoleInput) и выгребаем событийный мусор (ReadConsoleInput), пока
// в голове очереди не окажется настоящее нажатие.
func readInput(p []byte, timeoutMS int) (int, error) {
	h := os.Stdin.Fd()
	switch r, _, _ := procWaitForSingleObject.Call(h, uintptr(timeoutMS)); r {
	case waitObject0:
		// есть события — разберёмся ниже
	case waitTimeout:
		return 0, nil // тишина
	default:
		return 0, syscall.EINVAL // WAIT_FAILED: хендл мёртв — не крутить busy-loop
	}
	var rec inputRecord
	var n uint32
	for {
		r, _, _ := procPeekConsoleInput.Call(h, uintptr(unsafe.Pointer(&rec)), 1,
			uintptr(unsafe.Pointer(&n)))
		if r == 0 {
			return 0, syscall.EINVAL // Peek не дался — консоль закрыта?
		}
		if n == 0 {
			return 0, nil // очередь опустела — были одни мусорные события
		}
		if rec.keyProducesBytes() {
			return syscall.Read(syscall.Handle(h), p) // настоящее нажатие
		}
		// фокус/мышь/key-up/голый модификатор — выкинуть и посмотреть дальше
		if r, _, _ := procReadConsoleInput.Call(h, uintptr(unsafe.Pointer(&rec)), 1,
			uintptr(unsafe.Pointer(&n))); r == 0 {
			return 0, syscall.EINVAL
		}
	}
}
