// Пример 01 — скаляры, «enum», указатели, little-endian, InspectType.
//
// Урок главы: даже у int есть устройство. Целые лежат младшим байтом вперёд
// (little-endian), у указателя внутри — просто адрес, а «enum» в Go — это
// именованный целочисленный тип с константами iota.
package main

import (
	eye "github.com/vitikevich-landau/go_magic_eye"
)

// Rank — «enum» по-гошному: тип + iota-константы. В памяти — обычный uint8.
type Rank uint8

const (
	Peasant Rank = iota
	Squire
	KnightRank
	King
)

func main() {
	answer := 0x2A      // int: 8 байт на 64-битной платформе
	price := int16(-2)  // два байта: дополнительный код (0xfffe)
	grade := KnightRank // «enum» — на деле один байт
	pi := 3.1415926535  // float64: IEEE-754 в шестнадцати hex-цифрах
	ptr := &answer      // указатель: слово-адрес, GC его видит

	eye.Inspect(&answer, "ответ")
	eye.Inspect(&price, "цена со знаком")
	eye.Inspect(&grade, "звание (enum)")
	eye.Inspect(&pi, "π")
	eye.Inspect(&ptr, "указатель на ответ")

	// статика типа — объект не нужен
	eye.InspectType[Rank]("паспорт Rank")
}
