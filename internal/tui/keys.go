// Package tui — ядро терминального интерфейса странствия: декодер клавиш,
// кадр, цикл событий. Без внешних библиотек: байты stdin → события → ANSI.
package tui

import "unicode/utf8"

// KeyType — тип события клавиатуры.
type KeyType int

const (
	KRune KeyType = iota
	KUp
	KDown
	KLeft
	KRight
	KEnter
	KEsc
	KTab
	KBackspace
	KPgUp
	KPgDn
	KHome
	KEnd
	KF1
	KCtrlC
)

// Key — одно событие.
type Key struct {
	Type KeyType
	R    rune
}

// Decoder — конечный автомат: байты (кусками любого размера) → клавиши.
// Незавершённые ESC-последовательности ждут следующего куска; одинокий ESC
// добывается вызовом Flush по таймауту.
type Decoder struct{ buf []byte }

// Feed скармливает очередной кусок байтов и возвращает готовые клавиши.
func (d *Decoder) Feed(b []byte) []Key {
	d.buf = append(d.buf, b...)
	var out []Key
	for {
		k, n, ok := d.next()
		if !ok {
			break
		}
		d.buf = d.buf[n:]
		out = append(out, k)
	}
	return out
}

// Flush — таймаут: недоеденный одинокий ESC становится клавишей Esc.
func (d *Decoder) Flush() []Key {
	if len(d.buf) == 1 && d.buf[0] == 0x1b {
		d.buf = d.buf[:0]
		return []Key{{Type: KEsc}}
	}
	return nil
}

// next пытается снять одну клавишу с начала буфера.
func (d *Decoder) next() (Key, int, bool) {
	b := d.buf
	if len(b) == 0 {
		return Key{}, 0, false
	}
	switch b[0] {
	case 0x0d, 0x0a:
		return Key{Type: KEnter}, 1, true
	case 0x09:
		return Key{Type: KTab}, 1, true
	case 0x7f, 0x08:
		return Key{Type: KBackspace}, 1, true
	case 0x03:
		return Key{Type: KCtrlC}, 1, true
	case 0x1b:
		return d.escape()
	}
	if b[0] < 0x20 {
		return Key{Type: KRune, R: rune(b[0])}, 1, true // прочие control — как есть
	}
	if !utf8.FullRune(b) {
		return Key{}, 0, false // ждём хвост UTF-8
	}
	r, n := utf8.DecodeRune(b)
	return Key{Type: KRune, R: r}, n, true
}

// escape разбирает ESC-последовательности: CSI (ESC [ … буква) и SS3 (ESC O x).
func (d *Decoder) escape() (Key, int, bool) {
	b := d.buf
	if len(b) == 1 {
		return Key{}, 0, false // возможно, хвост в пути; Flush решит
	}
	switch b[1] {
	case '[':
		for i := 2; i < len(b); i++ {
			c := b[i]
			if c >= '0' && c <= '9' || c == ';' {
				continue
			}
			return csiKey(string(b[2:i]), c), i + 1, true
		}
		if len(b) > 16 { // мусорная последовательность — сбросить
			return Key{Type: KEsc}, len(b), true
		}
		return Key{}, 0, false
	case 'O':
		if len(b) < 3 {
			return Key{}, 0, false
		}
		switch b[2] {
		case 'A':
			return Key{Type: KUp}, 3, true
		case 'B':
			return Key{Type: KDown}, 3, true
		case 'C':
			return Key{Type: KRight}, 3, true
		case 'D':
			return Key{Type: KLeft}, 3, true
		case 'H':
			return Key{Type: KHome}, 3, true
		case 'F':
			return Key{Type: KEnd}, 3, true
		case 'P':
			return Key{Type: KF1}, 3, true
		}
		return Key{Type: KEsc}, 3, true
	default:
		// ESC + обычный байт: считаем одиноким ESC, байт оставляем
		return Key{Type: KEsc}, 1, true
	}
}

func csiKey(params string, final byte) Key {
	switch final {
	case 'A':
		return Key{Type: KUp}
	case 'B':
		return Key{Type: KDown}
	case 'C':
		return Key{Type: KRight}
	case 'D':
		return Key{Type: KLeft}
	case 'H':
		return Key{Type: KHome}
	case 'F':
		return Key{Type: KEnd}
	case 'Z':
		return Key{Type: KTab} // Shift-Tab — пусть тоже переключает фокус
	case '~':
		switch params {
		case "1", "7":
			return Key{Type: KHome}
		case "4", "8":
			return Key{Type: KEnd}
		case "5":
			return Key{Type: KPgUp}
		case "6":
			return Key{Type: KPgDn}
		case "11":
			return Key{Type: KF1}
		}
	case 'P':
		return Key{Type: KF1}
	}
	return Key{Type: KEsc}
}
