// Package eye — 👁 Око мага: визуальный инспектор объектов Go.
//
// Подключил один пакет — и смотришь внутрь значения: паспорт типа, карта
// памяти с offset'ами и padding-дырами, заголовки string/slice/map, анатомия
// interface-значений (itab — «vtable» Go), встраивание структур, память кучи
// на панелях-спутниках.
//
//	eye.Inspect(obj)              // полный осмотр (печать в stdout)
//	eye.Inspect(&obj, "казна")    // по указателю — оригинал на месте, со своей подписью
//	eye.InspectType[T]()          // только статика типа (объект не нужен)
//	eye.Finspect(w, obj, опции…)  // в свой io.Writer; настройки — опциями With*
//
//	eye.Explore(&obj)             // СТРАНСТВИЕ: интерактивный TUI-обозреватель
//	g := eye.NewGallery()         // несколько корней в одной сессии
//	g.Add(&knight, "рыцарь").Add(nums).AddType(eye.TypeOf[Config]())
//	err := g.Run()                // блокирует до выхода (q/Esc); Ctrl-C → eye.ErrInterrupted
//
// Рефлексия в Go встроена в язык, поэтому — в отличие от C++-предка
// (github.com/vitikevich-landau/magic_eye) — Оку не нужны макросы-реестры:
// reflect видит все поля, включая неэкспортированные, а unsafe позволяет
// прочитать их значения по живому адресу.
package eye

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/vitikevich-landau/go_magic_eye/internal/model"
	"github.com/vitikevich-landau/go_magic_eye/internal/render"
	"github.com/vitikevich-landau/go_magic_eye/internal/text"
)

// Inspect — полный осмотр объекта (печать в stdout).
//
// Передан указатель — Око смотрит на оригинал по живому адресу, без копий.
// Передано значение — оно уже упаковано в any: Око честно осматривает коробку
// интерфейса (и это само по себе первый урок).
//
// Настройки — переменными окружения EYE_*; программный вывод — Finspect.
func Inspect(obj any, label ...string) {
	cfg := loadConfig(WithLabel(first(label)))
	emit(model.Of(obj, cfg.label), cfg)
}

// InspectType — только статика типа: объект не нужен.
func InspectType[T any](label ...string) {
	cfg := loadConfig(WithLabel(first(label)))
	emit(model.OfType(reflect.TypeOf((*T)(nil)).Elem(), cfg.label), cfg)
}

// Finspect — как Inspect, но печатает в w: буфер теста, файл отчёта, пайп.
// Подпись и остальные настройки — опциями (WithLabel, WithColor, WithWidth…);
// опции сильнее переменных окружения. Автоцвет у не-терминального писателя
// выключен — в буфер идёт чистый текст.
func Finspect(w io.Writer, obj any, opts ...Option) {
	cfg := loadConfig(append([]Option{WithWriter(w)}, opts...)...)
	emit(model.Of(obj, cfg.label), cfg)
}

// FinspectType — как InspectType, но печатает в w. Настройки — опциями.
func FinspectType[T any](w io.Writer, opts ...Option) {
	cfg := loadConfig(append([]Option{WithWriter(w)}, opts...)...)
	emit(model.OfType(reflect.TypeOf((*T)(nil)).Elem(), cfg.label), cfg)
}

// TypeOf — маркер «типа без объекта» для галереи: g.Add(eye.TypeOf[Config]()).
func TypeOf[T any]() TypeMarker {
	return TypeMarker{t: reflect.TypeOf((*T)(nil)).Elem()}
}

// TypeMarker — непрозрачный маркер «тип без объекта»: создаётся TypeOf[T]()
// и понимается галереей (Add/AddType) — в осмотр попадает паспорт типа без
// живого значения.
type TypeMarker struct{ t reflect.Type }

func first(label []string) string {
	if len(label) > 0 {
		return label[0]
	}
	return ""
}

// emit — единая точка вывода одной модели: развилка «человеку или машине».
func emit(m *model.Model, cfg config) {
	if cfg.format == JSON {
		printJSON([]*model.Model{m}, cfg)
		return
	}
	printLines(render.Render(m, cfg.renderOptions()), cfg)
}

// printJSON — конверт с моделями в писатель. Цвет, ширина и центрирование
// машинного вида не касаются. Ошибка маршалинга здесь невозможна по
// построению (в DTO только строки и числа), но глотать её молча нельзя —
// уйдёт валидным JSON-объектом с полем error.
//
// EYE_JSON_FD=N — отдельный канал для конвертов: машинный вывод уходит в
// готовый файловый дескриптор N (его открывает родитель — playground даёт
// pipe через ExtraFiles), а stdout остаётся целиком человеку. Так конверт
// не смешивается с печатью программы и не гибнет под её потолками. Канал
// не дался (fd закрыт) — конверт не теряется: падаем в обычный писатель.
func printJSON(models []*model.Model, cfg config) {
	b, err := model.ToJSON(models)
	if err != nil {
		b = []byte(fmt.Sprintf("{\"eye_json_version\":%d,\"error\":%q}", model.JSONVersion, err.Error()))
	}
	// конверт начинается с чистой строки: перед Inspect могла быть печать
	// без завершающего \n (fmt.Print) — потребитель, режущий поток по
	// строкам, не должен получить склейку «хвост{конверт}»
	payload := append([]byte("\n"), b...)
	payload = append(payload, '\n')
	if fd := envInt("EYE_JSON_FD", 0); fd > 0 {
		if _, werr := jsonFD(fd).Write(payload); werr == nil {
			return
		}
	}
	cfg.out.Write(payload)
}

// jsonFDs — обёртки унаследованных дескрипторов, по одной на fd и НАВСЕГДА:
// os.NewFile вешает финализатор, и одноразовая обёртка после GC закрыла бы
// чужой дескриптор — второй Inspect молча падал бы в фолбэк.
var jsonFDs sync.Map // int → *os.File

func jsonFD(fd int) *os.File {
	if f, ok := jsonFDs.Load(fd); ok {
		return f.(*os.File)
	}
	f, _ := jsonFDs.LoadOrStore(fd, os.NewFile(uintptr(fd), "eye-json"))
	return f.(*os.File)
}

// printLines — вывод с центрированием (EYE_CENTER) по ширине экрана.
func printLines(lines []string, cfg config) {
	pad := ""
	if cfg.center {
		block := 0
		for _, l := range lines {
			if w := text.VisWidth(l); w > block {
				block = w
			}
		}
		if d := (cfg.width - block) / 2; d > 0 {
			pad = strings.Repeat(" ", d)
		}
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(pad)
		b.WriteString(l)
		b.WriteString("\n")
	}
	fmt.Fprint(cfg.out, b.String())
}
