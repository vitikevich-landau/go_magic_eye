// Пример 12 — атлас стандартной библиотеки: знакомые типы под Оком.
//
// Урок главы: стандартная библиотека сделана из тех же кирпичей. time.Time —
// три приватных поля (wall/ext/loc) с хитрой упаковкой монотонных часов;
// sync.Mutex — два маленьких целых (state и sema), вся магия — в atomic;
// sync.Once — то же самое: done uint32 + Mutex; strings.Builder носит
// указатель на себя (addr) для ловли копирования и []byte-буфер.
// Всё это — приватные поля чужих пакетов; рефлексии всё равно.
package main

import (
	"strings"
	"sync"
	"time"

	eye "github.com/vitikevich-landau/go_magic_eye"
)

func main() {
	now := time.Now() // wall+ext: две упакованные шкалы времени, loc — указатель
	eye.Inspect(&now, "time.Time")

	var mu sync.Mutex
	mu.Lock() // state станет ненулевым — залоченность видна прямо в байтах
	eye.Inspect(&mu, "sync.Mutex (залочен)")
	mu.Unlock()

	var once sync.Once
	once.Do(func() {}) // done=1 — «выстрелившая» Once отличается одним байтом
	eye.Inspect(&once, "sync.Once (сработавшая)")

	var b strings.Builder
	b.WriteString("ку") // addr указывает на саму b — так ловится копирование
	eye.Inspect(&b, "strings.Builder")
}
