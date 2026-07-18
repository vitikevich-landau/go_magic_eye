package tui

import "testing"

// FuzzDecoder — декодер клавиш не имеет права паниковать и копить буфер:
// байты приходят любым мусором и любыми кусками (рваные CSI, битый UTF-8).
func FuzzDecoder(f *testing.F) {
	f.Add([]byte("\x1b[A\x1b[5~q"), uint8(1))
	f.Add([]byte("жЖ\x03\x1bOP\x1b["), uint8(2))
	f.Add([]byte("\x1b\x1b[123;456~\x80\xff"), uint8(3))
	f.Fuzz(func(t *testing.T, data []byte, chunk uint8) {
		d := &Decoder{}
		step := int(chunk%7) + 1
		for i := 0; i < len(data); i += step {
			end := i + step
			if end > len(data) {
				end = len(data)
			}
			d.Feed(data[i:end])
		}
		d.Flush()
		if len(d.buf) > 8 {
			t.Fatalf("буфер декодера распух после Flush: %d байт", len(d.buf))
		}
	})
}
