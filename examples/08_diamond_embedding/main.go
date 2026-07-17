// Пример 08 — ромб встраивания: в Go он НЕ сливается.
//
// В C++ virtual-база в ромбе одна на всех (Око-предок помечало её «общая»).
// В Go virtual-баз нет: D{B; C}, где B{A} и C{A}, несёт ДВА независимых
// под-объекта A с разными offset'ами. Обращение d.hp неоднозначно и не
// скомпилируется — только d.B.hp или d.C.hp. Око показывает оба под-объекта.
package main

import (
	eye "github.com/vitikevich-landau/go_magic_eye"
)

type A struct {
	hp int32
}

type B struct {
	A
	bPower int16
}

type C struct {
	A
	cMagic int16
}

// D — ромб: две копии A, никакого слияния.
type D struct {
	B
	C
	name string
}

func main() {
	d := D{name: "ромб"}
	d.B.hp = 100 // только так: d.hp — ошибка компиляции (ambiguous selector)
	d.C.hp = 200 // вторая, НЕЗАВИСИМАЯ копия

	eye.Inspect(&d, "ромб встраивания")
	// В секции «встраивание» видно: main.A встречается дважды,
	// по разным offset'ам — сравни со «общей virtual-базой» C++-предка.
}
