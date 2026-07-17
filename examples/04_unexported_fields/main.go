// Пример 04 — неэкспортированные поля: рефлексия видит ВСЁ.
//
// В C++-предке этому служили макросы EYE_DESCRIBE (реестр «имя + указатель
// на член», собранный внутри класса). В Go макросы не нужны: reflect отдаёт
// имена и offset'ы даже неэкспортированных полей, а unsafe позволяет
// прочитать их значения по живому адресу. Единственный законный запрет —
// reflect.Value.Interface() на чужом поле; Око обходит его через
// reflect.NewAt (задокументированный приём).
package main

import (
	eye "github.com/vitikevich-landau/go_magic_eye"
	"github.com/vitikevich-landau/go_magic_eye/examples/04_unexported_fields/guild"
)

func main() {
	m := guild.NewMember("Мерлин", 99)
	// поля name/level/mana — приватные для пакета guild; Око их видит
	eye.Inspect(m, "член гильдии (чужой пакет, всё private)")
}
