package tui

import "testing"

func feed(t *testing.T, d *Decoder, bytes string) []Key {
	t.Helper()
	return d.Feed([]byte(bytes))
}

func TestArrowsAndBasics(t *testing.T) {
	d := &Decoder{}
	keys := feed(t, d, "\x1b[A\x1b[B\x1b[C\x1b[D\r\t")
	want := []KeyType{KUp, KDown, KRight, KLeft, KEnter, KTab}
	if len(keys) != len(want) {
		t.Fatalf("клавиш: %d, ждали %d (%v)", len(keys), len(want), keys)
	}
	for i, k := range keys {
		if k.Type != want[i] {
			t.Fatalf("клавиша %d: %v, ждали %v", i, k.Type, want[i])
		}
	}
}

func TestPgAndTilde(t *testing.T) {
	d := &Decoder{}
	keys := feed(t, d, "\x1b[5~\x1b[6~\x1b[H\x1b[F")
	want := []KeyType{KPgUp, KPgDn, KHome, KEnd}
	for i, k := range keys {
		if k.Type != want[i] {
			t.Fatalf("клавиша %d: %v", i, k.Type)
		}
	}
}

func TestUTF8Runes(t *testing.T) {
	d := &Decoder{}
	keys := feed(t, d, "qЖ")
	if len(keys) != 2 || keys[0].R != 'q' || keys[1].R != 'Ж' {
		t.Fatalf("руны: %v", keys)
	}
	// рваный UTF-8: первый байт руны отдельно от второго
	zh := []byte("Ж")
	if got := d.Feed(zh[:1]); len(got) != 0 {
		t.Fatalf("половина руны не должна дать клавишу: %v", got)
	}
	if got := d.Feed(zh[1:]); len(got) != 1 || got[0].R != 'Ж' {
		t.Fatalf("склейка руны: %v", got)
	}
}

func TestLoneEscViaFlush(t *testing.T) {
	d := &Decoder{}
	if got := feed(t, d, "\x1b"); len(got) != 0 {
		t.Fatalf("одинокий ESC должен ждать: %v", got)
	}
	if got := d.Flush(); len(got) != 1 || got[0].Type != KEsc {
		t.Fatalf("Flush должен добыть Esc: %v", got)
	}
}

func TestSplitCSI(t *testing.T) {
	d := &Decoder{}
	if got := feed(t, d, "\x1b["); len(got) != 0 {
		t.Fatalf("недоеденный CSI: %v", got)
	}
	if got := feed(t, d, "B"); len(got) != 1 || got[0].Type != KDown {
		t.Fatalf("склейка CSI: %v", got)
	}
}

func TestCtrlC(t *testing.T) {
	d := &Decoder{}
	if got := feed(t, d, "\x03"); len(got) != 1 || got[0].Type != KCtrlC {
		t.Fatalf("Ctrl-C: %v", got)
	}
}

// Delete/Insert/F5 и прочие нераспознанные последовательности — пустышки,
// а НЕ Esc: раньше Delete выходил из странствия.
func TestUnknownSequencesAreIgnoredNotEsc(t *testing.T) {
	d := &Decoder{}
	got := feed(t, d, "\x1b[3~\x1b[2~\x1b[15~\x1bOQ")
	if len(got) != 4 {
		t.Fatalf("клавиш: %d (%v)", len(got), got)
	}
	for i, k := range got {
		if k.Type != KIgnore {
			t.Fatalf("клавиша %d: %v, ждали KIgnore", i, k.Type)
		}
	}
}

// Оборванный CSI не съедает следующую клавишу: по Flush он дозревает в Esc,
// хвост разбирается заново.
func TestFlushDrainsBrokenCSI(t *testing.T) {
	d := &Decoder{}
	if got := feed(t, d, "\x1b["); len(got) != 0 {
		t.Fatalf("недоеденный CSI не должен дать клавиш: %v", got)
	}
	got := d.Flush()
	if len(got) != 2 || got[0].Type != KEsc || got[1].R != '[' {
		t.Fatalf("Flush оборванного CSI: %v", got)
	}
	// декодер жив: следующая клавиша работает
	if got := feed(t, d, "q"); len(got) != 1 || got[0].R != 'q' {
		t.Fatalf("после Flush декодер сломан: %v", got)
	}
}
