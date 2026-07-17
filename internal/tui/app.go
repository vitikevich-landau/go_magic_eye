package tui

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/vitikevich-landau/go_magic_eye/internal/nav"
	"github.com/vitikevich-landau/go_magic_eye/internal/render"
	"github.com/vitikevich-landau/go_magic_eye/internal/term"
)

// App — странствие: дерево слева, «Гримуар» (детали) справа, гид внизу.
type App struct {
	S *nav.Session

	W, H    int
	focus   int // 0 — дерево, 1 — детали
	panel   render.Panel
	treeTop int
	detTop  int
	full    bool

	searching bool
	query     []rune
	lastQuery string

	status  string
	help    bool
	snapN   int
	snapDir string

	prev  []string // последний нарисованный кадр (для построчного диффа)
	dirty bool     // кадр требует перерисовки

	detNode  *nav.Node // для какого узла собраны детали
	detPanel render.Panel
	detFull  bool
	detW     int
	detLines []string
}

func NewApp(s *nav.Session, snapDir string) *App {
	return &App{S: s, panel: render.PanelAll, snapDir: snapDir}
}

// Run — интерактивный цикл: raw-терминал, альтернативный экран,
// восстановление при любом исходе (defer + сигналы).
func (a *App) Run() error {
	restore, err := term.Raw()
	if err != nil {
		return err
	}
	cleanup := func() {
		term.ExitAlt(os.Stdout)
		restore()
	}
	defer cleanup()

	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-sig:
			cleanup()
			os.Exit(130) // терминал уже цел — можно уходить
		case <-done:
		}
	}()

	term.EnterAlt(os.Stdout)
	a.resize()
	a.dirty = true
	dec := &Decoder{}
	buf := make([]byte, 128)
	for {
		if a.dirty {
			a.draw(os.Stdout)
			a.dirty = false
		}
		n, err := term.ReadInput(buf, 100)
		if err != nil {
			return nil // stdin закрыт — тихо выходим, терминал восстановит defer
		}
		var keys []Key
		if n > 0 {
			keys = dec.Feed(buf[:n])
		} else {
			keys = dec.Flush() // одинокий ESC дозрел
			a.resize()         // тишина — удобный момент проверить размер окна
		}
		for _, k := range keys {
			a.dirty = true
			if a.Handle(k) {
				return nil
			}
		}
	}
}

func (a *App) resize() {
	if w, h, ok := term.Size(); ok && w >= 40 && h >= 8 {
		if w != a.W || h != a.H {
			a.W, a.H = w, h
			a.detTop = 0
			a.prev = nil // размер сменился — полный перерис с очисткой
			a.dirty = true
		}
		return
	}
	if a.W == 0 {
		a.W, a.H = 100, 32
	}
}

// Handle — одна клавиша; true = выйти из странствия.
func (a *App) Handle(k Key) bool {
	if a.searching {
		return a.handleSearch(k)
	}
	a.status = "" // сообщение прошлой клавиши погасло; обработчик поставит новое
	if a.help {
		// пока открыт свиток помощи, клавиши только закрывают его
		if k.Type == KEsc || k.Type == KF1 || k.Type == KCtrlC ||
			(k.Type == KRune && (k.R == 'q' || k.R == '?')) {
			a.help = false
		}
		return false
	}
	switch k.Type {
	case KCtrlC, KEsc:
		return true
	case KUp:
		a.up(1)
	case KDown:
		a.down(1)
	case KRight, KEnter:
		a.enter()
	case KLeft:
		a.collapse()
	case KTab:
		a.focus = 1 - a.focus
	case KBackspace:
		a.S.Back()
		a.status = ""
	case KPgUp:
		a.page(-1)
	case KPgDn:
		a.page(1)
	case KHome:
		a.home()
	case KEnd:
		a.end()
	case KF1:
		a.help = !a.help
	case KRune:
		return a.rune(k.R)
	}
	return false
}

func (a *App) rune(r rune) bool {
	switch r {
	case 'q':
		return true
	case 'k':
		a.up(1)
	case 'j':
		a.down(1)
	case 'l':
		a.enter()
	case 'h':
		a.collapse()
	case 'g':
		a.enter() // «перейти по указателю» — тот же вход
	case 'b':
		a.S.Back()
	case 'm':
		a.setPanel(render.PanelMem)
	case 'p':
		a.setPanel(render.PanelPass)
	case 'v':
		a.setPanel(render.PanelIface)
	case 'x':
		a.setPanel(render.PanelHex)
	case 'f':
		a.full = !a.full
		a.status = onOff("развёртка длинных регионов", a.full)
	case 'e':
		a.S.ExpandAll()
	case 'c':
		a.S.CollapseAll()
	case '/':
		a.searching = true
		a.query = a.query[:0]
	case 'n':
		a.searchNext(false)
	case 'N':
		a.searchNext(true)
	case 's':
		a.snapshot()
	case '?':
		a.help = !a.help
	}
	if r >= '1' && r <= '9' {
		a.S.JumpRoot(int(r - '1'))
	}
	return false
}

func onOff(what string, on bool) string {
	if on {
		return what + ": ВКЛ"
	}
	return what + ": выкл"
}

func (a *App) setPanel(p render.Panel) {
	if a.panel == p {
		a.panel = render.PanelAll // повторное нажатие возвращает полный осмотр
	} else {
		a.panel = p
	}
	a.detTop = 0
}

func (a *App) up(n int) {
	if a.focus == 0 {
		a.S.Move(-n)
		a.detTop = 0
	} else {
		a.detTop = maxI(0, a.detTop-n)
	}
}

func (a *App) down(n int) {
	if a.focus == 0 {
		a.S.Move(n)
		a.detTop = 0
	} else {
		a.detTop += n // потолок подрежет draw
	}
}

func (a *App) page(dir int) {
	step := maxI(1, a.zoneH()-2)
	if dir < 0 {
		a.up(step)
	} else {
		a.down(step)
	}
}

func (a *App) home() {
	if a.focus == 0 {
		a.S.Cursor = 0
		a.S.Move(0)
	} else {
		a.detTop = 0
	}
}

func (a *App) end() {
	if a.focus == 0 {
		a.S.Cursor = len(a.S.Visible()) - 1
		a.S.Move(0)
	} else {
		a.detTop = 1 << 30
	}
}

func (a *App) enter() {
	n := a.S.Current()
	if n == nil {
		return
	}
	if n.Refusal != "" {
		a.status = "⛔ " + n.Refusal
		return
	}
	if n.Cycle != nil {
		a.status = "⟲ прыжок к уже показанному узлу"
	}
	a.S.Enter()
	a.detTop = 0
}

func (a *App) collapse() {
	// как в C++-предке: ← из зоны деталей не трогает дерево, а возвращает
	// фокус в него — «свернулась выбранная позиция» пугает из правого окна
	if a.focus == 1 {
		a.focus = 0
		return
	}
	a.S.Collapse()
	a.detTop = 0
}

func (a *App) handleSearch(k Key) bool {
	switch k.Type {
	case KEnter:
		a.searching = false
		a.lastQuery = string(a.query)
		a.searchNext(false)
	case KEsc, KCtrlC:
		a.searching = false
	case KBackspace:
		if len(a.query) > 0 {
			a.query = a.query[:len(a.query)-1]
		}
	case KRune:
		a.query = append(a.query, k.R)
	}
	return false
}

func (a *App) searchNext(back bool) {
	if a.lastQuery == "" {
		a.status = "поиск пуст: нажми / и набери запрос"
		return
	}
	if a.S.Search(a.lastQuery, back) {
		a.status = "найдено: " + a.lastQuery + " (n/N — дальше/назад)"
		a.detTop = 0
	} else {
		a.status = "не найдено среди РАСКРЫТЫХ узлов: " + a.lastQuery
	}
}

// snapshot — кадр в файл чистым текстом (без ANSI).
func (a *App) snapshot() {
	a.snapN++
	name := fmt.Sprintf("eye_snap_%03d.txt", a.snapN)
	if a.snapDir != "" {
		name = filepath.Join(a.snapDir, name)
	}
	var b strings.Builder
	for _, l := range a.Frame() {
		b.WriteString(stripANSI(l))
		b.WriteString("\n")
	}
	if err := os.WriteFile(name, []byte(b.String()), 0o644); err != nil {
		a.status = "снимок не записался: " + err.Error()
		return
	}
	a.status = "📸 снимок: " + name
}

func stripANSI(s string) string {
	var b strings.Builder
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
		b.WriteRune(r)
	}
	return b.String()
}

func maxI(a, b int) int {
	if a > b {
		return a
	}
	return b
}
