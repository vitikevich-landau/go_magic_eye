package eye

import (
	"errors"
	"os"
	"reflect"
	"strings"

	"github.com/vitikevich-landau/go_magic_eye/internal/model"
	"github.com/vitikevich-landau/go_magic_eye/internal/nav"
	"github.com/vitikevich-landau/go_magic_eye/internal/proto"
	"github.com/vitikevich-landau/go_magic_eye/internal/render"
	"github.com/vitikevich-landau/go_magic_eye/internal/term"
	"github.com/vitikevich-landau/go_magic_eye/internal/tui"
)

// ErrInterrupted — странствие прервано сигналом (Ctrl-C/SIGTERM). Терминал к
// моменту возврата из Run уже восстановлен; решение «жить дальше или выйти» —
// за вызывающим, Око os.Exit не делает:
//
//	if errors.Is(err, eye.ErrInterrupted) {
//		os.Exit(130) // принятый для Unix код «убит Ctrl-C»
//	}
var ErrInterrupted = errors.New("странствие прервано сигналом (Ctrl-C/SIGTERM)")

// Explore — СТРАНСТВИЕ: интерактивный обозреватель одного объекта.
// Эквивалент NewGallery().Add(obj, label...).Run().
func Explore(obj any, label ...string) error {
	return NewGallery().Add(obj, label...).Run()
}

// Gallery — несколько корней в одной сессии странствия.
//
//	g := eye.NewGallery()
//	g.Add(&knight, "рыцарь").Add(nums).AddType(eye.TypeOf[Config]())
//	err := g.Run() // блокирует до выхода (q/Esc)
//
// Контракт: объекты галереи и всё достижимое из них живут, пока идёт Run() —
// Око смотрит на живую память и копий не делает (кроме значений map).
type Gallery struct {
	roots []galleryRoot
	opts  []Option
}

type galleryRoot struct {
	obj   any
	label string
	t     reflect.Type // тип без объекта (AddType)
}

// NewGallery — пустая галерея. Опции действуют на Run: WithWriter уводит
// статическую печать и кадры EYE_SCRIPT в свой писатель, остальные With*
// перекрывают одноимённые переменные окружения.
func NewGallery(opts ...Option) *Gallery { return &Gallery{opts: opts} }

// Add — живой корень. Маркер eye.TypeOf[T]() добавит «тип без объекта».
func (g *Gallery) Add(obj any, label ...string) *Gallery {
	l := first(label)
	if m, ok := obj.(TypeMarker); ok {
		g.roots = append(g.roots, galleryRoot{t: m.t, label: l})
		return g
	}
	g.roots = append(g.roots, galleryRoot{obj: obj, label: l})
	return g
}

// AddType — «тип без объекта»: g.AddType(eye.TypeOf[Config]()).
func (g *Gallery) AddType(m TypeMarker, label ...string) *Gallery {
	g.roots = append(g.roots, galleryRoot{t: m.t, label: first(label)})
	return g
}

// Run — странствие. Не-терминал (redirect, CI, WithWriter в буфер) или
// EYE_INTERACTIVE=0 — статическая печать всех корней; EYE_SCRIPT — кадры в
// писатель по клавишам из строки. Блокирует до выхода (q/Esc).
//
// Прерывание сигналом (Ctrl-C/SIGTERM) возвращает ErrInterrupted — код выхода
// процесса выбирает вызывающий, не библиотека. Если терминал не дался
// (странная консоль?), Run честно печатает статикой и возвращает исходную
// ошибку.
func (g *Gallery) Run() error {
	cfg := loadConfig(g.opts...)
	// сеансовый протокол — самый сильный режим: клиент (playground, плагин)
	// явно попросил живой диалог по stdin/stdout (см. internal/proto)
	if envBool("EYE_SESSION", false) {
		proto.Run(g.session(), os.Stdin, cfg.out)
		return nil
	}
	// машинный вид сильнее странствия и скрипта: программа, запущенная ради
	// JSON (playground, снапшот), должна отдать JSON, даже если в окружении
	// завалялся EYE_SCRIPT
	if cfg.format == JSON {
		printJSON(g.models(), cfg)
		return nil
	}
	if script := os.Getenv("EYE_SCRIPT"); script != "" {
		g.runScript(script, cfg)
		return nil
	}
	// TUI рисует в stdout — странствие возможно, только когда писатель и есть
	// stdout-терминал (и stdin терминал: клавиши читать откуда-то надо)
	interactive := envBool("EYE_INTERACTIVE", true) && cfg.out == os.Stdout &&
		cfg.isTerminal() && term.IsTerminal(os.Stdin.Fd())
	if !interactive {
		g.printAll(cfg)
		return nil
	}
	app := tui.NewApp(g.session(), os.Getenv("EYE_SNAP_DIR"))
	if err := app.Run(); err != nil {
		if errors.Is(err, tui.ErrInterrupted) {
			return ErrInterrupted
		}
		// терминал не дался — честно печатаем статикой, ошибку отдаём наверх
		g.printAll(cfg)
		return err
	}
	return nil
}

// session — граф странствия из корней галереи.
//
// Дисциплина адресуемости та же, что у Inspect: указатель → живой оригинал
// (v.Elem() адресуем — Око смотрит на память пользователя), значение →
// адресуемая коробка reflect.New (честная копия). Без адресуемости не
// работали бы ни чтение приватных полей (NewAt), ни байтовые дампы.
func (g *Gallery) session() *nav.Session {
	s := nav.NewSession()
	for _, r := range g.roots {
		if r.t != nil {
			s.AddTypeRoot(r.t, r.label)
			continue
		}
		rv := reflect.ValueOf(r.obj)
		if !rv.IsValid() {
			s.AddTypeRoot(reflect.TypeOf((*any)(nil)).Elem(), "nil")
			continue
		}
		// та же дисциплина, что у Inspect: указатель — живой оригинал,
		// значение — адресуемая коробка (и она честно помечена копией)
		if rv.Kind() == reflect.Pointer && !rv.IsNil() {
			s.AddRoot(rv.Elem(), r.label)
		} else {
			box := reflect.New(rv.Type())
			box.Elem().Set(rv)
			n := s.AddRoot(box.Elem(), r.label)
			n.Copied = "корень добавлен ПО ЗНАЧЕНИЮ: это коробка-копия (упаковка в any); " +
				"указатели внутри ведут к живым данным. Хочешь оригинал — Add(&объект)"
		}
	}
	return s
}

func (g *Gallery) printAll(cfg config) {
	for _, m := range g.models() {
		printLines(render.Render(m, cfg.renderOptions()), cfg)
	}
}

// models — модели всех корней в порядке добавления.
func (g *Gallery) models() []*model.Model {
	ms := make([]*model.Model, 0, len(g.roots))
	for _, r := range g.roots {
		if r.t != nil {
			ms = append(ms, model.OfType(r.t, r.label))
		} else {
			ms = append(ms, model.Of(r.obj, r.label))
		}
	}
	return ms
}

func (g *Gallery) runScript(script string, cfg config) {
	app := tui.NewApp(g.session(), os.Getenv("EYE_SNAP_DIR"))
	w := cfg.width
	if w > 120 {
		w = 120
	}
	app.RunScript(strings.Fields(script), cfg.out, w, envInt("EYE_HEIGHT", 40))
}
