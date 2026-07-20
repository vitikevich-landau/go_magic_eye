package text

import (
	"fmt"
	"strings"
)

// RuneWidth — экранная ширина руны: 2 для CJK/эмодзи, 0 для комбинирующих,
// иначе 1. Таблица намеренно маленькая: кириллица, латиница и box-drawing —
// основной алфавит Ока — все ширины 1.
func RuneWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 0x20 || (r >= 0x7f && r < 0xa0): // управляющие
		return 0
	case r >= 0x0300 && r <= 0x036f: // комбинирующие диакритики
		return 0
	case r >= 0x1100 && r <= 0x115f, // Hangul Jamo
		r >= 0x2e80 && r <= 0xa4cf, // CJK
		r >= 0xac00 && r <= 0xd7a3, // Hangul
		r >= 0xf900 && r <= 0xfaff,
		r >= 0xfe30 && r <= 0xfe4f,
		r >= 0xff00 && r <= 0xff60,
		r >= 0xffe0 && r <= 0xffe6,
		r >= 0x1f300 && r <= 0x1faff, // эмодзи
		r >= 0x20000 && r <= 0x3fffd:
		return 2
	}
	return 1
}

// VisWidth — экранная ширина строки без учёта ANSI-последовательностей.
//
// Это ЕДИНСТВЕННАЯ мера длины, которой пользуется вся вёрстка Ока: len(s)
// для строки с кириллицей и цветами врёт трижды (байты ≠ руны ≠ колонки ≠
// видимые символы). Управляющая последовательность считается законченной на
// первой латинской букве — этого достаточно для CSI-кодов цвета/курсора,
// которые Око само же и порождает.
func VisWidth(s string) int {
	w := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if r == 0x1b {
			inEsc = true
			continue
		}
		w += RuneWidth(r)
	}
	return w
}

// ClipVis обрезает строку до max экранных колонок, не разрывая
// ANSI-последовательности; если что-то отрезано, дописывает «…» и сброс цвета.
func ClipVis(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if VisWidth(s) <= max {
		return s
	}
	var b strings.Builder
	w := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			b.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if r == 0x1b {
			inEsc = true
			b.WriteRune(r)
			continue
		}
		rw := RuneWidth(r)
		if w+rw > max-1 { // резервируем колонку под «…»
			break
		}
		b.WriteRune(r)
		w += rw
	}
	if strings.Contains(s, "\x1b") {
		b.WriteString(CReset)
	}
	b.WriteString("…")
	return b.String()
}

// ClipVisMid обрезает СЕРЕДИНУ строки до max экранных колонок: «нача…онец».
// Хвост ценен не меньше начала: у дженерика он различает инстанциации
// (Cache[…]int]), у имени функции хранит имя метода. Строки с ANSI-кодами
// не поддерживаются — такие честно уходят в ClipVis (разрезать раскрашенную
// строку пополам, не порвав escape-последовательности, дороже, чем полезно).
func ClipVisMid(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if VisWidth(s) <= max {
		return s
	}
	if strings.Contains(s, "\x1b") {
		return ClipVis(s, max)
	}
	dots := Rune("…", "~")
	budget := max - VisWidth(dots)
	if budget < 2 {
		return ClipVis(s, max)
	}
	head := (budget + 1) / 2
	tail := budget - head
	r := []rune(s)
	var b strings.Builder
	w, i := 0, 0
	for ; i < len(r) && w+RuneWidth(r[i]) <= head; i++ {
		b.WriteRune(r[i])
		w += RuneWidth(r[i])
	}
	w, j := 0, len(r)
	for j > i && w+RuneWidth(r[j-1]) <= tail {
		j--
		w += RuneWidth(r[j])
	}
	b.WriteString(dots)
	b.WriteString(string(r[j:]))
	return b.String()
}

// PadVis дополняет строку пробелами справа до w экранных колонок.
func PadVis(s string, w int) string {
	d := w - VisWidth(s)
	if d <= 0 {
		return s
	}
	return s + strings.Repeat(" ", d)
}

// Line — строитель строки, который сам считает экранную ширину.
type Line struct {
	sb strings.Builder
	w  int
}

// Add дописывает s в стиле style (учитывая глобальный выключатель цвета).
func (l *Line) Add(style, s string) *Line {
	l.sb.WriteString(Paint(style, s))
	l.w += VisWidth(s)
	return l
}

// Addf — Add с форматированием.
func (l *Line) Addf(style, format string, a ...any) *Line {
	return l.Add(style, fmt.Sprintf(format, a...))
}

// Sp дописывает n пробелов.
func (l *Line) Sp(n int) *Line {
	if n > 0 {
		l.sb.WriteString(strings.Repeat(" ", n))
		l.w += n
	}
	return l
}

// PadTo добивает строку пробелами до w колонок.
func (l *Line) PadTo(w int) *Line { return l.Sp(w - l.w) }

// W — текущая экранная ширина строки.
func (l *Line) W() int { return l.w }

func (l *Line) String() string { return l.sb.String() }

// Rune — символ по роли с ASCII-запасным вариантом (EYE_ASCII=1).
func Rune(unicode, ascii string) string {
	if ASCIIOn() {
		return ascii
	}
	return unicode
}

// HexByte — байт как два hex-символа.
func HexByte(b byte) string { return fmt.Sprintf("%02x", b) }

// PrintableASCII — байт для ascii-колонки дампа: печатный или «·».
func PrintableASCII(b byte) string {
	if b >= 0x20 && b < 0x7f {
		return string(rune(b))
	}
	return Rune("·", ".")
}
