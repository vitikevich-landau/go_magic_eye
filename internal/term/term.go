// Package term — ОС-слой терминала: raw-режим, размер, альтернативный экран.
// Единственный пакет проекта, знающий про операционную систему.
//
// Три полноценные платформы: Linux и macOS (termios через syscall) и Windows
// (консоль kernel32 + VT-последовательности). На прочих ОС пакет честно
// отвечает «не терминал» — и Око деградирует в статическую печать.
package term

import "io"

// IsTerminal — является ли дескриптор интерактивным терминалом.
func IsTerminal(fd uintptr) bool { return isTerminal(fd) }

// Size — ширина и высота терминала stdout.
func Size() (w, h int, ok bool) { return size() }

// Raw переводит stdin в «сырой» режим (и готовит консоль Windows к ANSI).
// Возвращённая функция ВОССТАНАВЛИВАЕТ терминал; её обязаны позвать при
// любом исходе — за этим следит tui.App (defer + сигналы).
func Raw() (restore func(), err error) { return raw() }

// EnableColor готовит stdout к ANSI-цветам и отвечает, можно ли красить.
// На Unix терминал понимает ANSI всегда — true. На Windows цвета работают
// только после включения VIRTUAL_TERMINAL_PROCESSING: без этого статическая
// печать Inspect засыпала бы консоль мусором вида «←[38;5;117m». Включаем и
// не выключаем — это безвредно и так живёт сам Windows Terminal.
func EnableColor() bool { return enableColor() }

// ReadInput читает доступные байты stdin, ожидая не дольше ~timeoutMS.
// (0, nil) — тишина: цикл TUI использует паузу для Flush одинокого ESC и
// опроса размера окна. Работает только между Raw() и restore().
func ReadInput(p []byte, timeoutMS int) (int, error) { return readInput(p, timeoutMS) }

// Управление экраном — чистый ANSI, одинаковый на всех трёх платформах
// (Windows 10+ понимает VT после включения VIRTUAL_TERMINAL_PROCESSING).

func EnterAlt(w io.Writer) { io.WriteString(w, "\x1b[?1049h\x1b[?25l\x1b[2J\x1b[H") }
func ExitAlt(w io.Writer)  { io.WriteString(w, "\x1b[?25h\x1b[?1049l") }
