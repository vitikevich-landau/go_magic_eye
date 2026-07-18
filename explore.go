package eye

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/vitikevich-landau/go_magic_eye/internal/model"
	"github.com/vitikevich-landau/go_magic_eye/internal/nav"
	"github.com/vitikevich-landau/go_magic_eye/internal/render"
	"github.com/vitikevich-landau/go_magic_eye/internal/term"
	"github.com/vitikevich-landau/go_magic_eye/internal/tui"
)

// Explore — СТРАНСТВИЕ: интерактивный обозреватель одного объекта.
// Эквивалент NewGallery().Add(obj, label...).Run().
func Explore(obj any, label ...string) {
	NewGallery().Add(obj, label...).Run()
}

// Gallery — несколько корней в одной сессии странствия.
//
//	g := eye.NewGallery()
//	g.Add(&knight, "рыцарь").Add(nums).AddType(eye.TypeOf[Config]())
//	g.Run() // блокирует до выхода (q/Esc)
//
// Контракт: объекты галереи и всё достижимое из них живут, пока идёт Run() —
// Око смотрит на живую память и копий не делает (кроме значений map).
type Gallery struct {
	roots []galleryRoot
}

type galleryRoot struct {
	obj   any
	label string
	t     reflect.Type // тип без объекта (AddType)
}

// NewGallery — пустая галерея.
func NewGallery() *Gallery { return &Gallery{} }

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

// Run — странствие. Не-терминал (redirect, CI) или EYE_INTERACTIVE=0 —
// статическая печать всех корней; EYE_SCRIPT — кадры в stdout по клавишам
// из строки. Блокирует до выхода (q/Esc).
func (g *Gallery) Run() {
	cfg := loadConfig()
	if script := os.Getenv("EYE_SCRIPT"); script != "" {
		g.runScript(script, cfg)
		return
	}
	interactive := envBool("EYE_INTERACTIVE", true) &&
		term.IsTerminal(os.Stdout.Fd()) && term.IsTerminal(os.Stdin.Fd())
	if !interactive {
		g.printAll(cfg)
		return
	}
	app := tui.NewApp(g.session(), os.Getenv("EYE_SNAP_DIR"))
	if err := app.Run(); err != nil {
		if errors.Is(err, tui.ErrInterrupted) {
			// Ctrl-C/SIGTERM: терминал уже восстановлен цельным путём
			// (без гонок с отрисовкой) — уходим принятым для Unix кодом
			os.Exit(130)
		}
		// терминал не дался (странная консоль?) — честно печатаем статикой
		fmt.Fprintln(os.Stderr, "eye:", err)
		g.printAll(cfg)
	}
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
	for _, r := range g.roots {
		var m *model.Model
		if r.t != nil {
			m = model.OfType(r.t, r.label)
		} else {
			m = model.Of(r.obj, r.label)
		}
		printLines(render.Render(m, cfg.renderOptions()), cfg)
	}
}

func (g *Gallery) runScript(script string, cfg config) {
	app := tui.NewApp(g.session(), os.Getenv("EYE_SNAP_DIR"))
	w := cfg.width
	if w > 120 {
		w = 120
	}
	app.RunScript(strings.Fields(script), os.Stdout, w, envInt("EYE_HEIGHT", 40))
}
