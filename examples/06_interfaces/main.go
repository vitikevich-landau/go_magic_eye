// Пример 06 — интерфейсы: полиморфизм и «vtable» Go.
//
// Урок главы: в C++ vptr живёт В ОБЪЕКТЕ, в Go таблица методов (itab) живёт
// В ЗНАЧЕНИИ ИНТЕРФЕЙСА. Один и тот же объект стоит за разными интерфейсными
// значениями с разными itab. Бонус-ловушка: typed nil.
package main

import (
	"fmt"

	eye "github.com/vitikevich-landau/go_magic_eye"
)

type Speaker interface {
	Speak() string
	Volume() int
}

type Knight struct{ Name string }

func (k *Knight) Speak() string { return "За короля!" }
func (k *Knight) Volume() int   { return 11 }

type Mouse struct{ squeaks uint8 }

func (m *Mouse) Speak() string { return "пи-пи" }
func (m *Mouse) Volume() int   { return 1 }

func main() {
	k := Knight{Name: "Ланселот"}
	m := Mouse{squeaks: 3}

	// одна и та же интерфейсная переменная, два динамических типа —
	// Око показывает, как подменяются оба слова (tab и data)
	var s Speaker = &k
	eye.Inspect(&s, "голос: рыцарь")
	s = &m
	eye.Inspect(&s, "голос: мышь")

	// ЛОВУШКА: интерфейс с nil-указателем внутри — сам НЕ nil
	var ghost *Knight // nil
	s = ghost
	eye.Inspect(&s, "голос: призрак (typed nil)")
	fmt.Println("s == nil?", s == nil, "← вот почему «err != nil, а ошибки нет»")

	// пустой интерфейс (any): вместо itab — голый *_type
	var box any = 42
	eye.Inspect(&box, "коробка any")
}
