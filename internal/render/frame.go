// Package render — ВИД Ока: превращает model.Model в готовые строки.
// Ничего не знает о reflect: только модель, буквы и цвета.
package render

import (
	"strings"

	"github.com/vitikevich-landau/go_magic_eye/internal/text"
)

// Options — как рисовать.
type Options struct {
	Width int  // полная ширина рамки (колонок)
	Full  bool // не сворачивать длинные регионы (EYE_FULL=1)
	Bare  bool // без рамок (для панели деталей TUI, где своя рамка)
}

func (o Options) inner() int { return o.Width - 4 } // внутри «│ … │»

// глифы рамки с ASCII-запасом
func gTL() string { return text.Rune("╭", "+") }
func gTR() string { return text.Rune("╮", "+") }
func gBL() string { return text.Rune("╰", "+") }
func gBR() string { return text.Rune("╯", "+") }
func gH() string  { return text.Rune("─", "-") }
func gV() string  { return text.Rune("│", "|") }

// frame оборачивает готовые строки контента в рамку с картушем-заголовком.
func frame(title string, content []string, o Options) []string {
	w := o.Width
	in := o.inner()
	var out []string

	top := &text.Line{}
	top.Add(text.CFrame, gTL()+gH())
	if title != "" {
		top.Add(text.CFrame, text.Rune("‹ ", "[ "))
		top.Add(text.CTitle, title)
		top.Add(text.CFrame, text.Rune(" ›", " ]"))
	}
	rest := w - top.W() - 1
	if rest > 0 {
		top.Add(text.CFrame, strings.Repeat(gH(), rest))
	}
	top.Add(text.CFrame, gTR())
	out = append(out, top.String())

	for _, c := range content {
		if text.VisWidth(c) > in {
			c = text.ClipVis(c, in)
		}
		l := &text.Line{}
		l.Add(text.CFrame, gV()).Sp(1)
		l.Add("", c)
		l.Sp(in - text.VisWidth(c) + 1)
		l.Add(text.CFrame, gV())
		out = append(out, l.String())
	}

	bot := &text.Line{}
	bot.Add(text.CFrame, gBL()+strings.Repeat(gH(), w-2)+gBR())
	out = append(out, bot.String())
	return out
}

// section — внутренний заголовок секции: «── память ────…»
func section(name string, o Options) string {
	l := &text.Line{}
	l.Add(text.CFrame, strings.Repeat(gH(), 2)+" ")
	l.Add(text.CTitle, name)
	l.Add(text.CFrame, " "+strings.Repeat(gH(), max(0, o.inner()-l.W()-1)))
	return l.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
