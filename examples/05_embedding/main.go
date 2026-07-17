// Пример 05 — встраивание: «наследование» по-гошному.
//
// Урок главы: struct Knight { Unit; … } кладёт под-объект Unit по offset 0 —
// как базу в C++ — и продвигает его методы наружу. Но это НЕ наследование:
// нет виртуальности, нет иерархии типов — есть композиция и синтаксический
// сахар доступа к полям и методам.
package main

import (
	eye "github.com/vitikevich-landau/go_magic_eye"
)

// Unit — «база»: приватные поля, свой метод.
type Unit struct {
	hp    int32
	speed int32
}

func (u *Unit) Walk() string { return "топ-топ" }

// Knight — «наследник»: встраивает Unit значением.
type Knight struct {
	Unit          // под-объект по offset 0; Walk продвинут наружу
	armor  int16
	banner string
}

// Paladin — второй этаж: встраивание работает на любую глубину.
type Paladin struct {
	Knight
	halo bool
}

func main() {
	p := Paladin{
		Knight: Knight{Unit: Unit{hp: 100, speed: 5}, armor: 30, banner: "Грифон"},
		halo:   true,
	}
	// Око показывает: под-объекты с offset'ами, поля с пометкой «из какого
	// встроенного типа», продвинутые методы
	eye.Inspect(&p, "паладин")

	// доступ через встраивание — сахар: p.hp на деле p.Knight.Unit.hp
	_ = p.Walk()
}
