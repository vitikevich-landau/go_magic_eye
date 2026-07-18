// Package tui — ядро терминального интерфейса странствия: декодер клавиш,
// кадр, цикл событий. Без внешних библиотек: байты stdin → события → ANSI.
package tui

import "unicode/utf8"

// KeyType — тип события клавиатуры.
type KeyType int

const (
	// KIgnore — «клавиша-пустышка»: нераспознанная ESC-последовательность
	// (Delete, Insert, F2–F12, мышиные репорты…). Раньше такие декодировались
	// как Esc — и Delete закрывал всё странствие. Теперь они честно
	// проглатываются.
	KIgnore KeyType = iota
	KRune
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
//
// Почему это вообще автомат: терминал шлёт не «клавиши», а байты, и клавиша
// может приехать разрезанной на два read'а. Стрелка ↑ — это ТРИ байта
// «ESC [ A»; русская буква — два байта UTF-8; а одинокий байт ESC значит
// «пользователь нажал Esc»… либо «сейчас доедет хвост стрелки». Поэтому
// незавершённые последовательности ждут следующего куска в buf, а одинокий
// ESC добывается вызовом Flush по таймауту тика (~100 мс тишины = это
// точно был Esc, а не начало стрелки).
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

// Flush — таймаут тишины: недоеденная ESC-последовательность дозревает.
// Одинокий ESC становится клавишей Esc; оборванный «ESC [»/«ESC O» (терминал
// завис, Alt+скобка) — тоже Esc, а хвост разбирается заново как обычные
// байты. Без этого декодер застревал бы: следующая настоящая клавиша
// приклеивалась бы к огрызку и пропадала.
func (d *Decoder) Flush() []Key {
	if len(d.buf) == 0 || d.buf[0] != 0x1b {
		return nil
	}
	rest := append([]byte(nil), d.buf[1:]...)
	d.buf = d.buf[:0]
	keys := []Key{{Type: KEsc}}
	if len(rest) > 0 {
		keys = append(keys, d.Feed(rest)...)
	}
	return keys
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

// escape разбирает ESC-последовательности.
//
// Два семейства (наследие древних терминалов DEC VT100):
//
//	CSI: ESC [ <параметры-цифры-и-точки-с-запятой> <финальная буква>
//	     ESC[A — стрелка ↑, ESC[5~ — PgUp, ESC[1;5C — Ctrl+→ …
//	SS3: ESC O <буква> — те же стрелки в «application mode», F1-F4
//
// Финальная буква и решает, что это было; параметры до неё копятся.
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
		if len(b) > 16 { // мусорная последовательность — молча проглотить
			return Key{Type: KIgnore}, len(b), true
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
		return Key{Type: KIgnore}, 3, true // F2-F4 и прочее — не выход!
	default:
		// ESC + обычный байт (Alt+клавиша): считаем одиноким ESC, байт
		// оставляем — разберётся следующим заходом
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
	// Delete (3~), Insert (2~), F5-F12, Ctrl+стрелки с параметрами и весь
	// прочий зоопарк: у Ока для них нет действия. Раньше сюда возвращался
	// KEsc — и Delete ВЫХОДИЛ из странствия; теперь — пустышка.
	return Key{Type: KIgnore}
}
