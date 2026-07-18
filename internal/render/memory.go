package render

import (
	"fmt"
	"strings"

	"github.com/vitikevich-landau/go_magic_eye/internal/model"
	"github.com/vitikevich-landau/go_magic_eye/internal/text"
)

// Карта памяти — главная секция. Каждый регион (поле / дыра / служебное
// слово) — блоком строк: offset, «кирпичи», hex, ascii; сбоку выноска.
// Байт со смещением N всегда стоит в колонке N%8 — выравнивание видно глазами.

const bytesPerRow = 8

// memLayout — раскладка колонок карты памяти. Полный вид:
//
//	+0x0018  ████████  47 72 69 66 66 69 6e 00  Griffin·   ← banner : string = …
//	offset   кирпичи   hex (по 3 колонки на байт) ascii       выноска
//
// На узких экранах (панель деталей TUI) колонки отваливаются по одной:
// сначала ascii, потом кирпичи — hex и выноска не сдаются никогда.
type memLayout struct {
	bricks bool // колонка кирпичей
	ascii  bool // ascii-колонка
	left   int  // ширина левого блока до выносок
	callW  int  // ширина колонки выносок (для переносов)
}

func layoutFor(inner int) memLayout {
	l := memLayout{bricks: true, ascii: true}
	// 7 (offset) + 2 + 8 (кирпичи) + 2 + 24 (hex) + 2 + 8 (ascii) + 2
	l.left = 7 + 2 + 8 + 2 + 3*bytesPerRow + 2 + bytesPerRow + 2
	if inner-l.left < 24 { // выноске нужно хотя бы ~24 колонки
		l.ascii = false
		l.left -= bytesPerRow + 2
	}
	if inner-l.left < 24 {
		l.bricks = false
		l.left -= 10
	}
	return l
}

func memoryLines(m *model.Model, o Options) []string {
	lay := layoutFor(o.inner())
	lay.callW = max(16, o.inner()-lay.left-2)
	out := []string{memSummary(m)}
	for i := range m.Regions {
		out = append(out, regionBlock(m, &m.Regions[i], lay, o)...)
	}
	return out
}

func memSummary(m *model.Model) string {
	var fields, words int
	var holes uintptr
	for _, r := range m.Regions {
		switch r.Kind {
		case model.RPadding:
			holes += r.Size
		case model.RWord:
			words++
		default:
			fields++
		}
	}
	l := &text.Line{}
	l.Addf(text.CVal, "%d Б", m.Passport.Size)
	l.Addf(text.CName, " · полей %d", fields)
	if holes > 0 {
		l.Addf(text.CPad, " · дыр %d Б", holes)
	} else {
		l.Add(text.COk, " · дыр нет")
	}
	if !m.HasValue {
		l.Add(text.CNote, " · объекта нет — только статика типа")
	}
	return l.String()
}

// regionBlock — строки одного региона: слева байты, справа выноска.
func regionBlock(m *model.Model, r *model.Region, lay memLayout, o Options) []string {
	call := callout(r, lay.callW)
	if r.Size == 0 {
		l := &text.Line{}
		l.Sp(7+2).Add(text.CNote, "∅ ")
		return append([]string{}, zip(l.String(), call, lay)...)
	}

	first := int(r.Offset / bytesPerRow)
	last := int((r.Offset + r.Size - 1) / bytesPerRow)
	rows := make([]string, 0, last-first+1)
	for row := first; row <= last; row++ {
		rows = append(rows, byteRow(m, r, row, lay))
	}
	// свёртка длинных регионов
	if !o.Full && len(rows) > 4 {
		hidden := (len(rows) - 3) * bytesPerRow
		fold := (&text.Line{}).Sp(9).Addf(text.CNote, "%s ещё ~%d Б (f/EYE_FULL=1 развернёт) %s", text.Rune("⋯", "..."), hidden, text.Rune("⋯", "...")).String()
		rows = append(append(rows[:2:2], fold), rows[len(rows)-1])
	}
	return zipAll(rows, call, lay)
}

// byteRow — одна 8-байтовая строка региона: только его байты, чужие колонки
// пусты. Именно пустоты и учат: байт со смещением N всегда стоит в колонке
// N%8, поэтому поле, начинающееся с offset 4, «висит» посреди строки — и
// глаз сразу видит, как выравнивание раскидало поля по словам.
func byteRow(m *model.Model, r *model.Region, row int, lay memLayout) string {
	base := uintptr(row * bytesPerRow)
	l := &text.Line{}
	l.Addf(text.COff, "+0x%04x", base)
	l.Sp(2)

	inReg := func(i uintptr) bool { return i >= r.Offset && i < r.Offset+r.Size }
	if lay.bricks {
		for c := uintptr(0); c < bytesPerRow; c++ {
			if inReg(base + c) {
				l.Add(brickStyle(r.Kind), brickChar(r.Kind))
			} else {
				l.Sp(1)
			}
		}
		l.Sp(2)
	}
	var asc strings.Builder
	for c := uintptr(0); c < bytesPerRow; c++ {
		i := base + c
		if !inReg(i) {
			l.Sp(3)
			asc.WriteString(" ")
			continue
		}
		if m.HasValue && int(i) < len(m.Bytes) {
			l.Add(text.CHex, text.HexByte(m.Bytes[i])+" ")
			asc.WriteString(text.PrintableASCII(m.Bytes[i]))
		} else {
			l.Add(text.CNote, text.Rune("·· ", ".. "))
			asc.WriteString(" ")
		}
	}
	l.Sp(1)
	if lay.ascii {
		l.Add(text.CAscii, asc.String())
		l.Sp(2)
	}
	return l.String()
}

func brickChar(k model.RegionKind) string {
	switch k {
	case model.RPadding:
		return text.Rune("░", "~")
	case model.RWord:
		return text.Rune("▓", "=")
	}
	return text.Rune("█", "#")
}

func brickStyle(k model.RegionKind) string {
	switch k {
	case model.RPadding:
		return text.CPad
	case model.RWord:
		return text.CItab
	}
	return text.CName
}

// callout — строки выноски региона (заметки переносятся под ширину колонки).
func callout(r *model.Region, callW int) []string {
	var out []string
	if r.Kind == model.RPadding {
		l := &text.Line{}
		l.Add(text.CPad, fmt.Sprintf("%s дыра %d Б", text.Rune("⋯", "..."), r.Size))
		out = append(out, l.String())
		if r.Note != "" {
			out = append(out, wrapAt(2, text.Rune("↳ ", "-> "), r.Note, callW, text.CPad)...)
		}
		return out
	}
	l := &text.Line{}
	l.Add(text.CName, r.Name)
	l.Add(text.CFrame, " : ")
	l.Add(text.CType, r.TypeName)
	if r.Value != "" {
		l.Add(text.CFrame, " = ").Add(text.CVal, r.Value)
	}
	out = append(out, l.String())
	if r.Note != "" {
		out = append(out, wrapAt(2, text.Rune("↳ ", "-> "), r.Note, callW, text.CNote)...)
	}
	if r.From != "" {
		out = append(out, (&text.Line{}).Add(text.CNote, "  "+text.Rune("⌂ из ", "из ")+r.From).String())
	}
	return out
}

// zipAll склеивает левые строки (байты) и правые (выноску) бок о бок:
// блок региона высотой max(листы байтов, строки выноски); первая строка
// выноски получает стрелку «←», остальные выравниваются ПОД неё — отступ
// продолжений равен ширине стрелки (в ASCII она шире: «<- »).
func zipAll(left, right []string, lay memLayout) []string {
	arrow := text.Rune("← ", "<- ")
	aw := text.VisWidth(arrow)
	n := max(len(left), len(right))
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		l := &text.Line{}
		if i < len(left) {
			l.Add("", left[i])
		}
		l.PadTo(lay.left)
		if i < len(right) {
			if i == 0 {
				l.Add(text.CFrame, arrow)
			} else {
				l.Sp(aw)
			}
			l.Add("", right[i])
		}
		out = append(out, l.String())
	}
	return out
}

func zip(leftOne string, right []string, lay memLayout) []string {
	return zipAll([]string{leftOne}, right, lay)
}
