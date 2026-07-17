// Пример 03 — встроенные типы: string, срез, массив, map.
//
// Урок главы: string — это 2 слова (data+len), срез — 3 (data+len+cap),
// массив лежит в объекте ЦЕЛИКОМ, а map — одно слово-указатель на hmap.
// Всю «начинку», живущую вне объекта, Око выносит на панели-спутники.
package main

import (
	"strings"

	eye "github.com/vitikevich-landau/go_magic_eye"
)

type Vault struct {
	Motto   string         // литерал: буфер в .rodata, НЕ в куче
	Forged  string         // построенная строка: буфер в куче
	Coins   []int64        // len 3, cap 8: виден cap-хвост под append
	Runes   [4]rune        // массив: 16 байт прямо в структуре
	Ledger  map[string]int // слово-указатель на бакеты
}

func main() {
	coins := make([]int64, 0, 8)
	coins = append(coins, 100, 500, 1000)

	v := Vault{
		Motto:  "Semper fidelis",
		Forged: strings.Repeat("ku", 3) + "!", // куётся в рантайме → куча
		Coins:  coins,
		Runes:  [4]rune{'Р', 'У', 'Н', 'Ы'},
		Ledger: map[string]int{"меч": 1, "щит": 2, "эль": 33},
	}
	eye.Inspect(&v, "сокровищница")

	// пустой и nil — разные вещи: у nil-среза все три слова нулевые,
	// у пустого data может быть ненулевым
	var nilSlice []byte
	empty := []byte{}
	eye.Inspect(&nilSlice, "nil-срез")
	eye.Inspect(&empty, "пустой срез")
}
