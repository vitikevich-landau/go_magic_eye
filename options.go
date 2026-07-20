package eye

import (
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/vitikevich-landau/go_magic_eye/internal/render"
	"github.com/vitikevich-landau/go_magic_eye/internal/term"
	"github.com/vitikevich-landau/go_magic_eye/internal/text"
)

// Настройки Ока задаются двумя рычагами:
//
//   - опциями With* — программно, на конкретный вызов (Finspect, FinspectType,
//     NewGallery);
//   - переменными окружения EYE_* — извне, без перекомпиляции (те же рычаги,
//     что в C++-версии).
//
// Приоритет: опция > переменная окружения > автоматика (терминал/дефолт).
//
//	EYE_WIDTH=N        считать терминал N колонок (иначе определяем сами)
//	EYE_CENTER=0       не центрировать — прижать влево
//	EYE_COLOR=1/0      форсировать цвета вкл/выкл
//	EYE_FULL=1         не сворачивать длинные регионы
//	EYE_INTERACTIVE=0  Explore не входит в TUI — печатает как Inspect
//	EYE_SCRIPT="…"     исполнить клавиши (down enter q) и печатать кадры
//	EYE_HEIGHT=N       высота кадра в EYE_SCRIPT-режиме (по умолчанию 40)
//	EYE_ASCII=1        рамки и стрелки — чистый ASCII
//	EYE_SNAP_DIR=…     каталог для снимков экрана клавишей s
//	EYE_FORMAT=json    вместо рамок печатать JSON-модель (машинный вид)
//	EYE_JSON_FD=N      конверты JSON — в файловый дескриптор N (открывает
//	                   родитель), stdout остаётся человеку
//	EYE_SESSION=1      Explore/галерея: сеансовый JSON-протокол по
//	                   stdin/stdout вместо TUI (см. internal/proto)
//
// Кроме своих переменных Око уважает общепринятые сигналы: NO_COLOR
// (непустое значение — цвет выкл) и TERM=dumb; явный EYE_COLOR их
// перекрывает.

type config struct {
	out    io.Writer
	label  string
	width  int // 0 — определить по терминалу (не терминал — 100)
	center bool
	full   bool
	color  *bool // nil — автоматика: «писатель — терминал с ANSI?»
	ascii  bool
	format Format
}

// Format — вид, в котором Око отдаёт увиденное.
type Format int

const (
	// Text — рамки-картуши для человека (по умолчанию).
	Text Format = iota
	// JSON — конверт с моделями для машин: playground, снапшоты, диффы,
	// сторонние визуализации. Контракт — playground/SPEC.md §2.1. Цвет,
	// ширина и центрирование в этом виде не участвуют.
	JSON
)

// WithFormat — вид вывода: Text (рамки) или JSON (как EYE_FORMAT=json).
// В JSON-режиме Inspect/Finspect печатают конверт с одной моделью, галерея —
// конверт со всеми корнями; TUI-странствие не запускается.
func WithFormat(f Format) Option { return func(c *config) { c.format = f } }

// Option — программная настройка одного вызова (Finspect, FinspectType,
// NewGallery). Опция сильнее одноимённой переменной окружения.
type Option func(*config)

// WithWriter направляет вывод в w вместо os.Stdout: буфер теста, файл отчёта,
// пайп. Галерея с таким писателем печатает статикой — TUI живёт только на
// живом терминале.
func WithWriter(w io.Writer) Option {
	return func(c *config) {
		if w != nil {
			c.out = w
		}
	}
}

// WithLabel — подпись в заголовке (эквивалент label у Inspect). Действует на
// Finspect и FinspectType; галерея её игнорирует — там у каждого корня своя
// подпись, вторым аргументом Add/AddType.
func WithLabel(label string) Option { return func(c *config) { c.label = label } }

// WithWidth — считать экран шириной cols колонок (как EYE_WIDTH).
func WithWidth(cols int) Option {
	return func(c *config) {
		if cols > 0 {
			c.width = cols
		}
	}
}

// WithColor принудительно включает или выключает ANSI-цвета (как EYE_COLOR).
func WithColor(on bool) Option { return func(c *config) { c.color = &on } }

// WithASCII — рамки и стрелки чистым ASCII (как EYE_ASCII=1).
func WithASCII(on bool) Option { return func(c *config) { c.ascii = on } }

// WithFull — не сворачивать длинные регионы (как EYE_FULL=1).
func WithFull(on bool) Option { return func(c *config) { c.full = on } }

// WithCenter — центрировать блок по ширине экрана (как EYE_CENTER).
func WithCenter(on bool) Option { return func(c *config) { c.center = on } }

// envLookupBool — булева переменная и признак «задана ли вообще».
func envLookupBool(name string) (val, ok bool) {
	switch os.Getenv(name) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	}
	return false, false
}

func envBool(name string, def bool) bool {
	if v, ok := envLookupBool(name); ok {
		return v
	}
	return def
}

// envFormat — EYE_FORMAT: «json» — машинный вид, всё прочее (включая
// пустоту и «text») — рамки. Незнакомое значение не ошибка: человек получит
// привычный текст, а не молчание.
func envFormat(name string) Format {
	if strings.EqualFold(os.Getenv(name), "json") {
		return JSON
	}
	return Text
}

func envInt(name string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n > 0 {
		return n
	}
	return def
}

// loadConfig — раз за вызов: окружение могло смениться между Inspect'ами.
// Сначала окружение, поверх него опции, а что осталось на автомате —
// решается по итоговому писателю: цвет и ширина берутся у терминала, буферу
// достаются чистый текст и 100 колонок.
func loadConfig(opts ...Option) config {
	cfg := config{
		out:    os.Stdout,
		width:  envInt("EYE_WIDTH", 0),
		center: envBool("EYE_CENTER", true),
		full:   envBool("EYE_FULL", false),
		ascii:  envBool("EYE_ASCII", false),
		format: envFormat("EYE_FORMAT"),
	}
	if v, ok := envLookupBool("EYE_COLOR"); ok {
		cfg.color = &v
	}
	for _, o := range opts {
		o(&cfg)
	}

	// автоцвет = «писатель — терминал» И «терминал готов исполнять ANSI»:
	// на Windows второе требует включить VT-режим именно этого хэндла
	// (term.EnableColor делает это сам; stdout ≠ stderr — режимы у каждого
	// свои); WithColor/EYE_COLOR перекрывают автоматику в обе стороны.
	// Ниже автоматики, но выше «просто tty» стоят общепринятые сигналы
	// экосистемы: NO_COLOR (https://no-color.org — непустое значение
	// выключает цвет; ставят его в том числе люди с чувствительностью к
	// цвету) и TERM=dumb (терминал, честно объявивший, что ANSI не умеет).
	// Явный EYE_COLOR сильнее общих сигналов: конкретное бьёт общее.
	color := false
	fd, onTTY := cfg.terminalFd()
	switch {
	case cfg.color != nil:
		color = *cfg.color
		if color && onTTY {
			// цвет принудительный, но VT-режим Windows включить всё равно
			// надо — иначе legacy-conhost напечатает «←[38;5;…m» буквально
			term.EnableColor(fd)
		}
	case os.Getenv("NO_COLOR") != "":
		color = false
	case os.Getenv("TERM") == "dumb":
		color = false
	case onTTY:
		color = term.EnableColor(fd)
	}
	text.SetColor(color)
	text.SetASCII(cfg.ascii)

	// ширина — у того терминала, куда пойдёт вывод (не обязательно stdout)
	if cfg.width == 0 {
		cfg.width = 100
		if onTTY {
			if w, _, sized := term.Size(fd); sized {
				cfg.width = w
			}
		}
	}
	return cfg
}

// terminalFd — дескриптор писателя, если тот оказался живым терминалом.
func (c config) terminalFd() (uintptr, bool) {
	f, ok := c.out.(*os.File)
	if !ok || !term.IsTerminal(f.Fd()) {
		return 0, false
	}
	return f.Fd(), true
}

// isTerminal — итоговый писатель оказался живым терминалом.
func (c config) isTerminal() bool {
	_, ok := c.terminalFd()
	return ok
}

// renderOptions — ширина рамки: не шире экрана и не безумно широко.
func (c config) renderOptions() render.Options {
	w := c.width
	if w > 110 {
		w = 110
	}
	return render.Options{Width: w, Full: c.full}
}
