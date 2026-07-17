package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/vitikevich-landau/go_magic_eye/internal/nav"
	"github.com/vitikevich-landau/go_magic_eye/internal/render"
	"github.com/vitikevich-landau/go_magic_eye/internal/text"
)

// Кадр странствия:
//
//	строка 0      — картуш: галерея корней [1] [2] …
//	строка 1      — ярлыки зон: ▌ ДЕРЕВО ▐ │ ── детали (ответ на «где я» после Tab)
//	строки 2..H-2 — дерево │ детали («Гримуар»)
//	строка H-1    — гид по клавишам / строка поиска / статус
func (a *App) zoneH() int { return a.H - 3 }

func (a *App) treeW() int {
	w := a.W * 2 / 5
	if w < 28 {
		w = 28
	}
	if w > a.W-30 {
		w = a.W / 2
	}
	return w
}

// Frame — построить кадр: ровно H строк ширины W.
func (a *App) Frame() []string {
	out := make([]string, 0, a.H)
	out = append(out, a.titleBar())
	tw := a.treeW()
	dw := a.W - tw - 1
	out = append(out, a.zoneTabs(tw, dw))
	// разделитель подсвечен золотом, когда фокус в деталях — глаз сразу
	// видит, какая зона ловит ↑↓/PgUp (приём C++-предка)
	spine := text.CFrame
	if a.focus == 1 {
		spine = text.CFocus
	}
	tree := a.treeLines(tw)
	det := a.detailLines(dw)
	for i := 0; i < a.zoneH(); i++ {
		l := &text.Line{}
		if i < len(tree) {
			l.Add("", tree[i])
		}
		l.PadTo(tw)
		l.Add(spine, text.Rune("│", "|"))
		if i < len(det) {
			d := det[i]
			// зона деталей обрезается ЗДЕСЬ, на сборке кадра: строка шире
			// экрана = автоперенос = прокрутка = мерцание всего терминала
			if text.VisWidth(d) > dw {
				d = text.ClipVis(d, dw)
			}
			l.Add("", d)
		}
		l.PadTo(a.W)
		out = append(out, l.String())
	}
	out = append(out, a.statusBar())
	return out
}

func (a *App) titleBar() string {
	l := &text.Line{}
	l.Add(text.CTitle, text.Rune(" ◉ странствие Ока ", " (*) странствие Ока "))
	cur := a.S.Current()
	for i, r := range a.S.Roots {
		style := text.CNote
		if root(cur) == r {
			style = text.CTitle
		}
		l.Addf(style, " [%d]%s", i+1, r.Label)
	}
	if a.panel != render.PanelAll {
		l.Add(text.CItab, "  · панель: "+panelName(a.panel))
	}
	s := l.String()
	if text.VisWidth(s) > a.W {
		s = text.ClipVis(s, a.W)
	}
	return s
}

// zoneTabs — полоса ярлыков зон: активная — громкой плашкой-инверсией,
// пассивная — тихой строчной с линейкой. Главный ответ на «где я» после Tab.
func (a *App) zoneTabs(tw, dw int) string {
	l := &text.Line{}
	l.Add("", zoneTab("ДЕРЕВО", "дерево", a.focus == 0, tw))
	sep := text.CFrame
	if a.focus == 1 {
		sep = text.CFocus
	}
	l.Add(sep, text.Rune("│", "|"))
	l.Add("", zoneTab("ДЕТАЛИ", "детали", a.focus == 1, dw))
	return l.String()
}

func zoneTab(loud, quiet string, active bool, width int) string {
	rule := text.Rune("─", "-")
	var chip, style string
	if active {
		chip = text.Rune("▌ ", "[ ") + loud + text.Rune(" ▐", " ]")
		style = text.CSel
	} else {
		chip = rule + rule + " " + quiet + " "
		style = text.CNote
	}
	if text.VisWidth(chip) > width {
		chip = text.ClipVis(chip, width)
	}
	l := &text.Line{}
	l.Add(style, chip)
	if fill := width - l.W(); fill > 0 {
		l.Add(text.CFrame, strings.Repeat(rule, fill))
	}
	return l.String()
}

func root(n *nav.Node) *nav.Node {
	for n != nil && n.Parent != nil {
		n = n.Parent
	}
	return n
}

func panelName(p render.Panel) string {
	switch p {
	case render.PanelMem:
		return "память (m)"
	case render.PanelPass:
		return "паспорт (p)"
	case render.PanelIface:
		return "интерфейсы (v)"
	case render.PanelHex:
		return "hex (x)"
	}
	return "всё"
}

// ── дерево ──────────────────────────────────────────────────────────────────

func (a *App) treeLines(w int) []string {
	vis := a.S.Visible()
	h := a.zoneH()
	// прокрутка: курсор всегда в окне
	if a.S.Cursor < a.treeTop {
		a.treeTop = a.S.Cursor
	}
	if a.S.Cursor >= a.treeTop+h {
		a.treeTop = a.S.Cursor - h + 1
	}
	if a.treeTop > len(vis)-1 {
		a.treeTop = maxI(0, len(vis)-1)
	}
	var out []string
	for i := a.treeTop; i < len(vis) && len(out) < h; i++ {
		out = append(out, a.treeLine(vis[i], i == a.S.Cursor, w))
	}
	return out
}

func (a *App) treeLine(n *nav.Node, cursor bool, w int) string {
	l := &text.Line{}
	l.Sp(1 + n.Depth*2)
	exp, expStyle := expander(n)
	l.Add(expStyle, exp+" ")
	labStyle := text.CName
	if n.Parent == nil {
		labStyle = text.CTitle
	}
	l.Add(labStyle, n.Label)
	if n.Sub != "" {
		l.Add(text.CFrame, " — ").Add(text.CNote, n.Sub)
	}
	s := l.String()
	if text.VisWidth(s) > w {
		s = text.ClipVis(s, w)
	}
	if cursor {
		pad := maxI(0, w-text.VisWidth(s))
		marker := text.CSel
		if a.focus != 0 {
			marker = text.CDim + text.CSel
		}
		if text.Color {
			return marker + stripANSI(s) + strings.Repeat(" ", pad) + text.CReset
		}
		return text.Rune("▶", ">") + s[minI(len(s), 1):]
	}
	return s
}

func expander(n *nav.Node) (string, string) {
	switch {
	case n.Cycle != nil:
		return text.Rune("⟲", "@"), text.CItab
	case n.Refusal != "":
		// не «⛔»: у эмодзи-глифов двойная ширина в большинстве шрифтов —
		// они сдвигают колонки дерева и снимков
		return text.Rune("✗", "x"), text.CWarn
	case n.Expanded:
		return text.Rune("▾", "-"), text.CFrame
	case n.HasKids():
		return text.Rune("▸", "+"), text.CFrame
	}
	return text.Rune("·", "."), text.CFrame
}

// ── детали («Гримуар») ──────────────────────────────────────────────────────

func (a *App) detailLines(w int) []string {
	if a.help {
		return helpLines()
	}
	n := a.S.Current()
	if n == nil {
		return nil
	}
	if a.detNode != n || a.detPanel != a.panel || a.detFull != a.full || a.detW != w {
		opts := render.Options{Width: w - 2, Full: a.full}
		a.detLines = render.RenderPanel(n.Detail(), opts, a.panel)
		a.detNode, a.detPanel, a.detFull, a.detW = n, a.panel, a.full, w
	}
	h := a.zoneH()
	if a.detTop > len(a.detLines)-h {
		a.detTop = maxI(0, len(a.detLines)-h)
	}
	var out []string
	for i := a.detTop; i < len(a.detLines) && len(out) < h; i++ {
		s := " " + a.detLines[i]
		if text.VisWidth(s) > w {
			s = text.ClipVis(s, w)
		}
		out = append(out, s)
	}
	// индикатор прокрутки — СТРОГО внутри ширины зоны: строка длиннее
	// терминала вызывает автоперенос, прокрутку и мерцание всего экрана
	if len(a.detLines) > h && len(out) > 0 {
		pos := fmt.Sprintf(" %d/%d ", a.detTop+1, len(a.detLines))
		keep := w - text.VisWidth(pos)
		first := out[0]
		if text.VisWidth(first) > keep {
			first = text.ClipVis(first, keep)
		}
		out[0] = text.PadVis(first, keep) + text.Paint(text.CNote, pos)
	}
	return out
}

// ── нижняя строка ───────────────────────────────────────────────────────────

func (a *App) statusBar() string {
	l := &text.Line{}
	switch {
	case a.searching:
		l.Add(text.CTitle, " / ").Add(text.CVal, string(a.query))
		l.Add(text.CNote, "▁  (Enter — искать, Esc — отмена)")
	case a.status != "":
		l.Add(text.CVal, " "+a.status)
		l.Add(text.CNote, "   · ? помощь · q выход")
	default:
		focus := "[дерево]"
		hintLeft := "← свернуть"
		if a.focus == 1 {
			focus = "[детали]"
			hintLeft = "← в дерево"
		}
		l.Add(text.CNote, " ↑↓ ходить · Enter/→ раскрыть/перейти · "+hintLeft+
			" · b назад · Tab ")
		l.Add(text.CFocus, focus)
		l.Add(text.CNote, " · m p v x панель · f развернуть · / поиск · s снимок · ? помощь · q выход")
	}
	s := l.String()
	if text.VisWidth(s) > a.W {
		s = text.ClipVis(s, a.W)
	}
	return s
}

func helpLines() []string {
	raw := []string{
		"╔═ свиток помощи ═╗",
		"",
		"  ↑↓ (k/j)      курсор по дереву",
		"  →/Enter (l)   раскрыть узел · перейти по указателю",
		"  ← (h)         свернуть · подняться; из деталей — фокус в дерево",
		"  g / b, ⌫      перейти по указателю / назад по истории",
		"  Tab           фокус: дерево ↔ детали (ярлык-плашка показывает где ты;",
		"                ↑↓/PgUp/PgDn листают фокусную зону)",
		"  m p v x       панель: память / паспорт / интерфейсы / hex",
		"  f · e · c     развернуть регионы · раскрыть ветку · свернуть всё",
		"  1..9          прыжок к N-му корню галереи",
		"  /, n, N       поиск по раскрытым узлам, дальше/назад",
		"  s             снимок в файл: дерево + детали узла (чистый текст)",
		"  ?/F1 · q/Esc  помощь · выход",
		"",
		"  Переходы типизированные. Честные отказы: nil, unsafe.Pointer",
		"  (тип стёрт), func (код, не данные). Узел-цикл помечен ⟲.",
		"  Объекты галереи живут, пока идёт Run() — Око смотрит на",
		"  живую память и копий не делает (кроме значений map).",
	}
	out := make([]string, len(raw))
	for i, s := range raw {
		out[i] = text.Paint(text.CNote, s)
	}
	return out
}

// ── вывод ───────────────────────────────────────────────────────────────────

// draw — построчный дифф: переписываются только изменившиеся строки, кадр
// обёрнут в «синхронизированный вывод» (DECSET 2026 — терминалы, которые
// его умеют, показывают кадр атомарно; остальные молча игнорируют).
func (a *App) draw(w io.Writer) {
	frame := a.Frame()
	var b strings.Builder
	b.WriteString("\x1b[?2026h")
	if len(a.prev) != len(frame) {
		b.WriteString("\x1b[2J")
		a.prev = make([]string, len(frame))
	}
	for i, l := range frame {
		if a.prev[i] == l {
			continue
		}
		fmt.Fprintf(&b, "\x1b[%d;1H", i+1)
		b.WriteString(l)
		b.WriteString("\x1b[K")
		a.prev[i] = l
	}
	b.WriteString("\x1b[?2026l")
	io.WriteString(w, b.String())
}

func minI(a, b int) int {
	if a < b {
		return a
	}
	return b
}
