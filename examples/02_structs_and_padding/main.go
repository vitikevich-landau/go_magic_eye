// Пример 02 — структуры и охота на padding.
//
// Go не переставляет поля: раскладка структуры — в руках автора.
// Око показывает каждую дыру и объясняет, кто её потребовал.
package main

import (
	eye "github.com/vitikevich-landau/go_magic_eye"
)

// Sloppy — поля свалены как попало: 3 дыры.
type Sloppy struct {
	Flag  bool    // 1 Б … и тут же дыра в 7 Б
	Coins int64   // хочет адрес, кратный 8
	Tag   byte    // 1 Б … и снова дыра
	Price float64 // опять кратно 8
	Level int16   // хвост доберёт до кратного 8
}

// Tidy — те же поля, отсортированы по убыванию выравнивания: дыр нет.
type Tidy struct {
	Coins int64
	Price float64
	Level int16
	Flag  bool
	Tag   byte
}

func main() {
	s := Sloppy{Flag: true, Coins: 777, Tag: 7, Price: 99.5, Level: 3}
	t := Tidy{Coins: 777, Price: 99.5, Level: 3, Flag: true, Tag: 7}

	eye.Inspect(&s, "неряха")
	eye.Inspect(&t, "аккуратист")
}
