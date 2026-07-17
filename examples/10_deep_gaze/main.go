// Пример 10 — глубокий взор: указатели трёх судеб, куча, каналы, функции,
// длинные массивы со свёрткой.
//
// Урок главы: указатель бывает живым (Око идёт по нему), nil (честный отказ)
// и стёртым (unsafe.Pointer — тип неизвестен, чужую память не читаем).
// Escape analysis решает, где живёт объект — а GC держит всё, до чего
// дотягиваются слова-указатели.
package main

import (
	eye "github.com/vitikevich-landau/go_magic_eye"
)

type Scroll struct {
	Rune  byte
	Power int32
}

type Lair struct {
	Alive   *Scroll        // живой: спутник покажет цель, странствие пройдёт
	Nobody  *Scroll        // nil: честный отказ
	Erased  uintptr        // голое число-адрес: GC его НЕ охраняет
	Signal  chan int       // слово → hchan (кольцевой буфер + очереди)
	Spell   func(int) int  // слово → funcval; имя достанем через FuncForPC
	Grimoire [64]byte      // длинный регион — свернётся «⋯ ещё N Б ⋯»
}

func double(x int) int { return x * 2 }

func main() {
	s := &Scroll{Rune: 'R', Power: 9000} // уходит в кучу: на него смотрят другие
	l := Lair{
		Alive:  s,
		Erased: 0xDEADBEEF,
		Signal: make(chan int, 4),
		Spell:  double,
	}
	l.Signal <- 1
	l.Signal <- 2
	for i := range l.Grimoire {
		l.Grimoire[i] = byte(i)
	}

	eye.Inspect(&l, "логово")
	// EYE_FULL=1 развернёт свёрнутый Grimoire целиком
}
