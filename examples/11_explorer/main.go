// Пример 11 — СТРАНСТВИЕ: интерактивный обозреватель, переходы по
// указателям, галерея. Запусти в терминале — и ходи стрелками.
package main

import (
	eye "github.com/vitikevich-landau/go_magic_eye"
)

type Loot struct {
	Gold  int
	Gems  []string
}

type Unit struct {
	hp, speed int32
}

type Knight struct {
	Unit
	Name   string
	Bag    *Loot
	Friend *Knight
	Deeds  map[string]int
}

type Config struct {
	Debug   bool
	Retries int
	Tag     string
}

func main() {
	bag := Loot{Gold: 1200, Gems: []string{"рубин", "изумруд", "сапфир"}}
	lancelot := Knight{Unit: Unit{hp: 100, speed: 7}, Name: "Ланселот", Bag: &bag}
	galahad := Knight{Unit: Unit{hp: 90, speed: 9}, Name: "Галахад", Bag: &bag,
		Friend: &lancelot, Deeds: map[string]int{"драконы": 2, "турниры": 17}}
	lancelot.Friend = &galahad // цикл: Око покажет ⟲ вместо дубля

	nums := make([]int, 0, 8)
	nums = append(nums, 3, 1, 4, 1, 5)

	g := eye.NewGallery()
	g.Add(&galahad, "Галахад").Add(&lancelot, "Ланселот").Add(nums, "числа")
	g.AddType(eye.TypeOf[Config](), "конфиг (тип)")
	g.Run()
}
