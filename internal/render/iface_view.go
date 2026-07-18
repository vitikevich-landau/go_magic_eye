package render

import (
	"fmt"
	"strings"

	"github.com/vitikevich-landau/go_magic_eye/internal/model"
	"github.com/vitikevich-landau/go_magic_eye/internal/text"
)

// Диаграмма interface-значения: «объект → два слова → itab → слоты → код».
// Прямой родственник vtable-блока из C++-версии, с той разницей, которую
// Око не устанет повторять: таблица методов в Go живёт в ЗНАЧЕНИИ интерфейса,
// а не в объекте — у объекта нет vptr.

func ifaceLines(m *model.Model, o Options) []string {
	var out []string
	for i := range m.Ifaces {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, oneIface(&m.Ifaces[i], o)...)
	}
	return out
}

func oneIface(f *model.Iface, o Options) []string {
	var out []string

	h := &text.Line{}
	h.Add(text.CName, f.Where)
	h.Add(text.CFrame, " — ")
	h.Add(text.CType, f.TypeName)
	if f.Empty {
		h.Add(text.CNote, " (eface: методов нет)")
	} else {
		h.Addf(text.CNote, " (iface: методов %d)", len(f.Methods))
	}
	out = append(out, h.String())

	arrow := text.Rune(" ─→ ", " -> ")
	w0 := &text.Line{}
	label := "tab "
	if f.Empty {
		label = "type"
	}
	w0.Add(text.CFrame, "  [0] ").Add(text.CItab, label).Sp(1)
	if f.TabAddr == 0 {
		w0.Add(text.CWarn, "0x0 (nil)")
	} else {
		w0.Addf(text.CAddr, "0x%x", f.TabAddr)
		if !f.Empty {
			w0.Add(text.CFrame, arrow)
			w0.Addf(text.CItab, "itab · hash 0x%x", f.Hash)
		} else {
			w0.Add(text.CFrame, arrow)
			w0.Add(text.CItab, "*_type — описание типа "+f.DynType)
		}
	}
	out = append(out, w0.String())

	w1 := &text.Line{}
	w1.Add(text.CFrame, "  [1] ").Add(text.CItab, "data").Sp(1)
	if f.DataAddr == 0 {
		w1.Add(text.CWarn, "0x0")
		if f.TypedNil {
			w1.Add(text.CWarn, " ← typed nil: тип есть, данных нет")
		}
	} else {
		w1.Addf(text.CAddr, "0x%x", f.DataAddr)
		w1.Add(text.CFrame, arrow)
		w1.Add(text.CType, f.DynType)
	}
	out = append(out, w1.String())

	if !f.Empty && f.TabAddr != 0 {
		nameW := 0
		for _, mt := range f.Methods {
			nameW = max(nameW, text.VisWidth(mt.Name))
		}
		arrow2 := text.Rune(" → ", " -> ")
		for i, mt := range f.Methods {
			prefix := fmt.Sprintf("      слот %d: ", i)
			l := &text.Line{}
			l.Add(text.CFrame, prefix)
			l.Add(text.CItab, mt.Name)
			if mt.PC == 0 {
				l.Sp(nameW - text.VisWidth(mt.Name))
				l.Add(text.CNote, " (слот не читали)")
				out = append(out, l.String())
				continue
			}
			addr := fmt.Sprintf(" @ 0x%x", mt.PC)
			pad := nameW - text.VisWidth(mt.Name)
			if l.W()+pad+text.VisWidth(arrow2)+text.VisWidth(mt.Func)+text.VisWidth(addr) <= o.inner() {
				l.Sp(pad)
				l.Add(text.CFrame, arrow2)
				l.Add(text.CVal, mt.Func)
				l.Add(text.CAddr, addr)
				out = append(out, l.String())
				continue
			}
			// имя функции длиннее строки: PC — то, ради чего диаграмма
			// существует, — остаётся на первой строке; имя уходит на
			// продолжение (при нужде — со сжатой серединой import-пути)
			l.Add(text.CAddr, addr)
			out = append(out, l.String())
			c := (&text.Line{}).Sp(text.VisWidth(prefix))
			c.Add(text.CFrame, text.Rune("↳ ", "-> "))
			fn := mt.Func
			if b := o.inner() - c.W(); text.VisWidth(fn) > b {
				fn = text.ClipVisMid(fn, b)
			}
			c.Add(text.CVal, fn)
			out = append(out, c.String())
		}
	}
	if f.Note != "" {
		style := text.CNote
		if f.TypedNil || strings.Contains(f.Note, "ловушка") {
			style = text.CWarn
		}
		out = append(out, wrapAt(2, text.Rune("✦ ", "* "), f.Note, o.inner(), style)...)
	}
	return out
}
