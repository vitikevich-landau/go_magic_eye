package render

import (
	"fmt"
	"strings"

	"github.com/vitikevich-landau/go_magic_eye/internal/model"
	"github.com/vitikevich-landau/go_magic_eye/internal/text"
)

// Panel — какая часть осмотра нужна (панели деталей странствия).
type Panel int

const (
	PanelAll Panel = iota
	PanelMem
	PanelPass
	PanelIface
	PanelHex
)

// RenderPanel — осмотр одной панели (для деталей TUI).
func RenderPanel(m *model.Model, o Options, p Panel) []string {
	if o.Width < 44 {
		o.Width = 44
	}
	title := m.Passport.TypeName
	if m.Label != "" {
		title = m.Label + " — " + title
	}
	var body []string
	switch p {
	case PanelAll:
		return Render(m, o)
	case PanelMem:
		body = append(body, section("память", o))
		body = append(body, memoryLines(m, o)...)
		out := frame(title, body, o)
		for i := range m.Sats {
			out = append(out, satLines(&m.Sats[i], o)...)
		}
		return out
	case PanelPass:
		body = append(body, passportLines(m, o)...)
		if len(m.Embeds) > 0 {
			body = append(body, "", section("встраивание", o))
			body = append(body, embedLines(m, o)...)
		}
		for _, n := range m.Notes {
			body = append(body, wrap("✦ ", n, o.inner(), text.CNote)...)
		}
	case PanelIface:
		if len(m.Ifaces) == 0 {
			body = append(body, (&text.Line{}).Add(text.CNote,
				"интерфейсов в этом узле нет — нечего показывать").String())
		} else {
			body = append(body, ifaceLines(m, o)...)
		}
	case PanelHex:
		if len(m.Bytes) == 0 {
			body = append(body, (&text.Line{}).Add(text.CNote,
				"байтов нет (объекта нет или он нулевого размера)").String())
		} else {
			body = append(body, hexDump(m.Bytes, 0, o)...)
		}
	}
	return frame(title, body, o)
}

// Render — весь осмотр: главная рамка с секциями + панели-спутники.
func Render(m *model.Model, o Options) []string {
	if o.Width < 56 {
		o.Width = 56
	}
	// вырожденная модель (узел-цикл, nil): только заметки, без пустых секций
	if m.Passport.TypeName == "" && len(m.Regions) == 0 {
		var body []string
		for _, n := range m.Notes {
			body = append(body, wrap("✦ ", n, o.inner(), text.CNote)...)
		}
		return frame(m.Label, body, o)
	}
	var body []string
	body = append(body, passportLines(m, o)...)
	if len(m.Embeds) > 0 {
		body = append(body, "", section("встраивание (композиция вместо наследования)", o))
		body = append(body, embedLines(m, o)...)
	}
	body = append(body, "", section("память", o))
	body = append(body, memoryLines(m, o)...)
	if len(m.Ifaces) > 0 {
		body = append(body, "", section("интерфейсы — «vtable» Go живёт в значении", o))
		body = append(body, ifaceLines(m, o)...)
	}
	if len(m.Notes) > 0 {
		body = append(body, "", section("свиток заметок", o))
		for _, n := range m.Notes {
			body = append(body, wrap("✦ ", n, o.inner(), text.CNote)...)
		}
	}

	title := m.Passport.TypeName
	if m.Label != "" {
		title = m.Label + " — " + title
	}
	var out []string
	if o.Bare {
		out = body
	} else {
		out = frame(title, body, o)
	}
	for i := range m.Sats {
		out = append(out, satLines(&m.Sats[i], o)...)
	}
	return out
}

// ── паспорт ─────────────────────────────────────────────────────────────────

func passportLines(m *model.Model, o Options) []string {
	p := m.Passport
	var out []string
	l := &text.Line{}
	l.Add(text.CName, "размер ").Addf(text.CVal, "%d Б", p.Size)
	l.Add(text.CFrame, " · ").Add(text.CName, "выравнивание ").Addf(text.CVal, "%d", p.Align)
	l.Add(text.CFrame, " · ").Add(text.CType, p.Kind)
	out = append(out, l.String())
	if m.HasValue && m.Addr != 0 {
		a := &text.Line{}
		a.Add(text.CName, "адрес ").Addf(text.CAddr, "0x%x", m.Addr)
		out = append(out, a.String())
	}
	for _, tr := range p.Traits {
		style := text.CNote
		if strings.HasPrefix(tr, "НЕ ") {
			style = text.CWarn
		}
		out = append(out, (&text.Line{}).Add(text.CFrame, "  • ").Add(style, tr).String())
	}
	return out
}

// ── встраивание ─────────────────────────────────────────────────────────────

func embedLines(m *model.Model, o Options) []string {
	var out []string
	for _, e := range m.Embeds {
		l := &text.Line{}
		l.Sp(e.Depth * 2)
		l.Add(text.CFrame, text.Rune("▸ ", "> "))
		l.Add(text.CType, e.TypeName)
		l.Addf(text.COff, " @ +%d", e.Offset)
		l.Addf(text.CNote, ", %d Б", e.Size)
		out = append(out, l.String())
		if len(e.Promoted) > 0 {
			p := &text.Line{}
			p.Sp(e.Depth*2 + 2)
			p.Add(text.CNote, "методы наружу: ").Add(text.CItab, strings.Join(e.Promoted, ", "))
			out = append(out, p.String())
		}
		if e.Note != "" {
			out = append(out, wrapAt(e.Depth*2+2, "⚠ ", e.Note, o.inner(), text.CWarn)...)
		}
	}
	return out
}

// ── спутники ────────────────────────────────────────────────────────────────

func satLines(s *model.Satellite, o Options) []string {
	var body []string
	h := &text.Line{}
	h.Add(text.CAddr, fmt.Sprintf("@ 0x%x", s.Addr))
	if s.Size > 0 {
		h.Addf(text.CNote, " · %d Б", s.Size)
	}
	body = append(body, h.String())
	if len(s.Bytes) > 0 {
		body = append(body, hexDump(s.Bytes, uintptr(0), o)...)
	}
	for _, e := range s.Elems {
		body = append(body, (&text.Line{}).Add(text.CVal, "  "+e).String())
	}
	if s.Note != "" {
		body = append(body, wrap("✦ ", s.Note, o.inner(), text.CNote)...)
	}
	so := o
	so.Width = o.Width - 4
	out := frame(text.Rune("☄ ", "* ")+s.Title, body, so)
	for i := range out {
		out[i] = "    " + out[i]
	}
	return out
}

// hexDump — простой дамп по 16 байт (для спутников).
func hexDump(b []byte, base uintptr, o Options) []string {
	const perRow = 16
	rows := (len(b) + perRow - 1) / perRow
	limit := rows
	if !o.Full && rows > 4 {
		limit = 3
	}
	var out []string
	for r := 0; r < limit; r++ {
		l := &text.Line{}
		l.Addf(text.COff, "  +%03x  ", base+uintptr(r*perRow))
		var asc strings.Builder
		for c := 0; c < perRow; c++ {
			i := r*perRow + c
			if i < len(b) {
				l.Add(text.CHex, text.HexByte(b[i])+" ")
				asc.WriteString(text.PrintableASCII(b[i]))
			} else {
				l.Sp(3)
				asc.WriteString(" ")
			}
			if c == 7 {
				l.Sp(1)
			}
		}
		l.Sp(1).Add(text.CAscii, asc.String())
		out = append(out, l.String())
	}
	if limit < rows {
		out = append(out, (&text.Line{}).Addf(text.CNote, "  ⋯ ещё %d Б (EYE_FULL=1 покажет всё) ⋯",
			len(b)-limit*perRow).String())
	}
	return out
}

// ── переносы ────────────────────────────────────────────────────────────────

// wrap — текст с префиксом, перенесённый по словам под ширину w.
func wrap(prefix, s string, w int, style string) []string {
	return wrapAt(0, prefix, s, w, style)
}

func wrapAt(indent int, prefix, s string, w int, style string) []string {
	avail := w - indent - text.VisWidth(prefix)
	if avail < 16 {
		avail = 16
	}
	words := strings.Fields(s)
	var lines []string
	cur := ""
	for _, wd := range words {
		if cur != "" && text.VisWidth(cur)+1+text.VisWidth(wd) > avail {
			lines = append(lines, cur)
			cur = wd
			continue
		}
		if cur == "" {
			cur = wd
		} else {
			cur += " " + wd
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	var out []string
	for i, ln := range lines {
		l := &text.Line{}
		l.Sp(indent)
		if i == 0 {
			l.Add(text.CFrame, prefix)
		} else {
			l.Sp(text.VisWidth(prefix))
		}
		l.Add(style, ln)
		out = append(out, l.String())
	}
	return out
}
