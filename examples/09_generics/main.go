// Пример 09 — дженерики: статический полиморфизм без itab.
//
// Аналог главы про CRTP из C++-предка: там статический полиморфизм давал
// вызовы без vptr. В Go дженерик Stack[T] порождает конкретные типы:
// Stack[int64] и Stack[string] — РАЗНЫЕ типы с разными размерами и
// раскладкой, и никакого слова-таблицы в объекте нет.
// (Внутри компилятор делит инстанцирования по gcshape и носит словари,
// но в ПАМЯТИ ОБЪЕКТА от этого ничего нет — смотри сам.)
package main

import (
	eye "github.com/vitikevich-landau/go_magic_eye"
)

// Stack — обобщённый стек.
type Stack[T any] struct {
	items []T
	top   int32
}

func (s *Stack[T]) Push(v T) {
	s.items = append(s.items, v)
	s.top++
}

func main() {
	var ints Stack[int64]
	ints.Push(7)
	ints.Push(77)

	var words Stack[string]
	words.Push("меч")
	words.Push("щит")

	// два инстанцирования — два самостоятельных типа
	eye.Inspect(&ints, "Stack[int64]")
	eye.Inspect(&words, "Stack[string]")
	eye.InspectType[Stack[[8]byte]]("Stack[[8]byte] — только тип")
}
