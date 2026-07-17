// Package eye — 👁 Око мага: визуальный инспектор объектов Go.
//
// Подключил один пакет — и смотришь внутрь значения: паспорт типа, карта
// памяти с offset'ами и padding-дырами, заголовки string/slice/map, анатомия
// interface-значений (itab — «vtable» Go), встраивание структур, память кучи
// на панелях-спутниках.
//
//	eye.Inspect(obj)              // полный осмотр (печать)
//	eye.Inspect(&obj, "казна")    // по указателю — оригинал на месте, со своей подписью
//	eye.InspectType[T]()          // только статика типа (объект не нужен)
//
//	eye.Explore(&obj)             // СТРАНСТВИЕ: интерактивный TUI-обозреватель
//	g := eye.NewGallery()         // несколько корней в одной сессии
//	g.Add(&knight, "рыцарь").Add(nums).AddType(eye.TypeOf[Config]())
//	g.Run()                       // блокирует до выхода (q/Esc)
//
// Рефлексия в Go встроена в язык, поэтому — в отличие от C++-предка
// (github.com/vitikevich-landau/magic_eye) — Оку не нужны макросы-реестры:
// reflect видит все поля, включая неэкспортированные, а unsafe позволяет
// прочитать их значения по живому адресу.
package eye

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/vitikevich-landau/go_magic_eye/internal/model"
	"github.com/vitikevich-landau/go_magic_eye/internal/render"
	"github.com/vitikevich-landau/go_magic_eye/internal/text"
)

// Inspect — полный осмотр объекта (печать в stdout).
//
// Передан указатель — Око смотрит на оригинал по живому адресу, без копий.
// Передано значение — оно уже упаковано в any: Око честно осматривает коробку
// интерфейса (и это само по себе первый урок).
func Inspect(obj any, label ...string) {
	cfg := loadConfig()
	m := model.Of(obj, first(label))
	printLines(render.Render(m, cfg.renderOptions()), cfg)
}

// InspectType — только статика типа: объект не нужен.
func InspectType[T any](label ...string) {
	cfg := loadConfig()
	m := model.OfType(reflect.TypeOf((*T)(nil)).Elem(), first(label))
	printLines(render.Render(m, cfg.renderOptions()), cfg)
}

// TypeOf — маркер «типа без объекта» для галереи: g.Add(eye.TypeOf[Config]()).
func TypeOf[T any]() TypeMarker {
	return TypeMarker{t: reflect.TypeOf((*T)(nil)).Elem()}
}

// TypeMarker — см. TypeOf.
type TypeMarker struct{ t reflect.Type }

func first(label []string) string {
	if len(label) > 0 {
		return label[0]
	}
	return ""
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
	fmt.Fprint(os.Stdout, b.String())
}
