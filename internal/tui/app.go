package tui

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/vitikevich-landau/go_magic_eye/internal/nav"
	"github.com/vitikevich-landau/go_magic_eye/internal/render"
	"github.com/vitikevich-landau/go_magic_eye/internal/term"
	"github.com/vitikevich-landau/go_magic_eye/internal/text"
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

// ErrInterrupted — странствие прервано сигналом (Ctrl-C/SIGTERM). Терминал
// к моменту возврата уже восстановлен; решение «жить дальше или выйти» — за
// вызывающим (eye.Gallery.Run отдаёт его наверх как eye.ErrInterrupted, код
// выхода выбирает программа-хозяин).
var ErrInterrupted = errors.New("странствие прервано сигналом")

// Run — интерактивный цикл: raw-терминал, альтернативный экран,
// восстановление при любом исходе (defer + сигналы).
//
// Устройство цикла: горутин-читателей НЕТ. term.ReadInput возвращается сам
// не позже ~100 мс (termios VTIME на Unix, WaitForSingleObject на Windows),
// поэтому один цикл успевает всё: забрать байты клавиш, по тишине дозреть
// одинокий Esc (Flush) и заметить смену размера окна. Побочная выгода —
// после выхода не остаётся «застрявшего» читателя, крадущего stdin у
// программы-хозяина.
func (a *App) Run() error {
	restore, err := term.Raw()
	if err != nil {
		return err
	}
	defer func() {
		term.ExitAlt(os.Stdout)
		restore()
	}()

	// Сигнал НЕ восстанавливает терминал сам — иначе горутина сигнала
	// гонялась бы с draw() главного цикла (запись в уже восстановленный
	// экран, двойной restore). Вместо этого она только поднимает флаг;
	// цикл замечает его не позже чем через ~100 мс (таймаут ReadInput,
	// а на Unix сигнал ещё и рвёт read немедленно — EINTR) и выходит
	// обычным путём, где defer'ы отработают в одном потоке.
	var interrupted atomic.Bool
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-sig:
			interrupted.Store(true)
		case <-done:
		}
	}()

	term.EnterAlt(os.Stdout)
	a.resize()
	a.dirty = true
	dec := &Decoder{}
	buf := make([]byte, 128)
	for {
		if interrupted.Load() {
			return ErrInterrupted
		}
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
			keys = dec.Flush() // одинокая/оборванная ESC-последовательность дозрела
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

// resize перечитывает размер окна. Принимается ЛЮБОЙ размер: если рисовать
// кадр по устаревшему большему, строки перенесутся и экран разъедется;
// про «слишком мало» честно скажет сам Frame.
func (a *App) resize() {
	w, h, ok := term.Size(os.Stdout.Fd()) // TUI рисует в stdout
	if !ok {
		if a.W == 0 {
			a.W, a.H = 100, 32
		}
		return
	}
	if w != a.W || h != a.H {
		a.W, a.H = w, h
		a.detTop = 0
		a.prev = nil // размер сменился — полный перерис с очисткой
		a.dirty = true
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
	case KIgnore:
		return false // нераспознанная последовательность — не действие
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
		a.snapshot() // точная копия экрана
	case 'S':
		a.snapshotDoc() // развёрнутый документ: дерево + детали целиком
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
	if n.Cycle != nil {
		a.status = text.Rune("⟲", "@") + " прыжок к уже показанному узлу"
		a.S.Enter()
		a.detTop = 0
		return
	}
	// тупик? — честный отказ ГОЛОСОМ, а не молчанием (Explain знает причину
	// и до ленивого построения детей)
	if !n.HasKids() {
		if msg := n.Explain(); msg != "" {
			a.status = text.Rune("✗", "x") + " " + msg
		}
		return
	}
	a.S.Enter()
	// дети могли построиться только что — если строитель записал отказ,
	// показать и его
	if n.Refusal != "" {
		a.status = text.Rune("✗", "x") + " " + n.Refusal
	}
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

// ── снимки (s / S) ──────────────────────────────────────────────────────────
//
// Две ипостаси, обе — чистый текст без ANSI:
//
//	s — ТОЧНАЯ КОПИЯ ЭКРАНА: последний показанный кадр, строка в строку,
//	    колонка в колонку. Единственная вольность — хвостовые пробелы
//	    каждой строки срезаны: они невидимы на экране, но в редакторе
//	    заворачивали бы строки и рушили вёрстку (урок C++-предка).
//	    Ширина файла = ширина терминала в момент снимка.
//	S — РАЗВЁРНУТЫЙ ДОКУМЕНТ: дерево целиком (без обрезки под зону) +
//	    детали текущего узла целиком, свёрстанные в фиксированные 100
//	    колонок — для отчётов и чтения вне терминала.

// snapWidth — ширина деталей в документе-снимке (S).
const snapWidth = 100

// snapshot (клавиша s) — точная копия экрана.
// Источник — a.prev: ровно те строки, что сейчас стоят на экране (их рисовал
// draw). Кадр из Frame() строится только если prev ещё пуст (script-режим).
func (a *App) snapshot() {
	rows := a.prev
	if len(rows) == 0 {
		rows = a.Frame()
	}
	var b strings.Builder
	for _, l := range rows {
		b.WriteString(strings.TrimRight(stripANSI(l), " "))
		b.WriteByte('\n')
	}
	a.writeSnap(b.String(), "снимок экрана")
}

// snapshotDoc (клавиша S) — развёрнутый документ: шапка, всё дерево с
// маркером курсора, полные детали текущего узла.
func (a *App) snapshotDoc() {
	var b strings.Builder
	line := func(s string) {
		b.WriteString(strings.TrimRight(stripANSI(s), " "))
		b.WriteByte('\n')
	}
	rule := text.Rune("─", "-")
	section := func(title string) {
		line("")
		line(rule + rule + " " + title + " " +
			strings.Repeat(rule, maxI(4, snapWidth-6-text.VisWidth(title))))
	}

	line(text.Rune("◉ ", "(*) ") + "странствие Ока — снимок")
	section("дерево")
	for i, n := range a.S.Visible() {
		cursor := "  "
		if i == a.S.Cursor {
			cursor = text.Rune("▶ ", "> ")
		}
		exp, _ := expander(n)
		s := cursor + strings.Repeat(" ", n.Depth*2) + exp + " " + n.Label
		if n.Sub != "" {
			s += " — " + n.Sub
		}
		line(s)
	}
	if cur := a.S.Current(); cur != nil {
		section("детали: " + cur.Label + " · панель: " + panelName(a.panel))
		det := render.RenderPanel(cur.Detail(), render.Options{Width: snapWidth, Full: a.full}, a.panel)
		for _, l := range det {
			line(l)
		}
	}
	a.writeSnap(b.String(), "снимок-документ")
}

// writeSnap пишет содержимое в первое свободное eye_snap_NNN.txt.
// Имя занимается АТОМАРНО (O_EXCL — «только создать, не усекать»): два
// процесса в одном каталоге не затирают снимки друг друга, а проверка
// «Stat, потом создать» страдала бы гонкой TOCTOU. Успех объявляется только
// после Close: полный диск всплывает на сбросе буфера, не на записи.
func (a *App) writeSnap(content, what string) {
	for n := a.snapN + 1; n <= 999; n++ {
		name := fmt.Sprintf("eye_snap_%03d.txt", n)
		if a.snapDir != "" {
			name = filepath.Join(a.snapDir, name)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if os.IsExist(err) {
				continue // имя занято — пробуем следующий номер
			}
			a.status = "снимок не записался: " + err.Error()
			return
		}
		_, werr := f.WriteString(content)
		cerr := f.Close()
		if werr != nil || cerr != nil {
			os.Remove(name) // не оставлять пустой огрызок
			a.status = "снимок не записался (место на диске?)"
			return
		}
		a.snapN = n
		a.status = text.Rune("📸", "[snap]") + " " + what + ": " + name
		return
	}
	a.status = "все 999 имён снимков заняты — почисти каталог"
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
