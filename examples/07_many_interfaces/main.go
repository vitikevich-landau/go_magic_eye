// Пример 07 — один тип, много интерфейсов: аналог множественного наследования.
//
// В C++ объект с двумя полиморфными базами носит ДВА vptr и offset-to-top.
// В Go объект не носит ничего: на каждый пары (тип, интерфейс) рантайм
// создаёт ОТДЕЛЬНЫЙ itab, а data всегда указывает на объект целиком —
// «шаг назад к началу объекта» не нужен по построению.
package main

import (
	eye "github.com/vitikevich-landau/go_magic_eye"
)

type Fighter interface{ Attack() int }
type Singer interface{ Sing() string }
type Bard interface { // «ромб» интерфейсов: композиция, не наследование
	Fighter
	Singer
}

type Hero struct {
	Name  string
	Power int
}

func (h *Hero) Attack() int  { return h.Power }
func (h *Hero) Sing() string { return "🎵 о доблести" }

func main() {
	h := Hero{Name: "Тарьен", Power: 7}

	// три интерфейсных значения — ТРИ разных itab, ОДИН data
	var f Fighter = &h
	var s Singer = &h
	var b Bard = &h

	type Stage struct { // все три рядом, чтобы сравнить tab-слова глазами
		F Fighter
		S Singer
		B Bard
	}
	st := Stage{F: f, S: s, B: b}
	eye.Inspect(&st, "сцена: три роли одного героя")
}
