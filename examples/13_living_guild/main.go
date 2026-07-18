// Пример 13 — ЖИВАЯ ГИЛЬДИЯ: финальная глава, где всё сразу.
//
// Горутины-работники разбирают заказы из канала, лочат мьютекс, пишут в
// журнал и копят золото — ПОКА ты ходишь по гильдии Оком. Двигай курсор и
// смотри, как меняются байты: len канала тает, Gold растёт, журнал длиннеет,
// state мьютекса мигает между 0 и 1.
//
// Что тут изучать странствием (Enter/→ по узлам):
//   - Heroes []Hero — срез ИНТЕРФЕЙСОВ: у каждого элемента свой itab (v);
//   - Board chan Quest — слово *hchan, len/cap живут и меняются;
//   - mu sync.Mutex — два маленьких инта, вся магия в atomic;
//   - Log []string — cap-хвост растёт скачками (append удваивает);
//   - Ranks map[string]int — порядок обхода каждый запуск разный.
//
// Честное предупреждение: Око подглядывает в память БЕЗ блокировок — для
// урока это и нужно, но детектор гонок (-race) справедливо возмутится.
// Это инструмент наблюдения, а не паттерн для продакшена.
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	eye "github.com/vitikevich-landau/go_magic_eye"
)

type Quest struct {
	ID     int32
	Reward int64
	Title  string
}

// Hero — интерфейс: в срезе гильдии у каждого героя будет свой itab.
type Hero interface {
	Do(q Quest) string
}

type Warrior struct{ Name string }
type Mage struct {
	Name string
	Mana int32
}

func (w *Warrior) Do(q Quest) string { return w.Name + " рубит: " + q.Title }
func (m *Mage) Do(q Quest) string    { m.Mana--; return m.Name + " колдует: " + q.Title }

// Guild — общее состояние: всё, что обычно прячут за абстракциями,
// здесь выставлено под Око.
type Guild struct {
	mu     sync.Mutex     // охраняет Log и Ranks
	Gold   atomic.Int64   // без мьютекса: одно атомарное слово
	Board  chan Quest     // доска заказов: буфер тает на глазах
	Heroes []Hero         // срез интерфейсов — разные itab, один слайс
	Log    []string       // растёт под мьютексом: смотри cap-скачки
	Ranks  map[string]int // выполненные заказы по героям
}

// worker — горутина-работник: канал → мьютекс → общее состояние.
func worker(g *Guild, h Hero, name string, wg *sync.WaitGroup) {
	defer wg.Done()
	for q := range g.Board {
		time.Sleep(150 * time.Millisecond) // нарочно медленно: смотри вживую
		g.Gold.Add(q.Reward)
		g.mu.Lock()
		g.Log = append(g.Log, h.Do(q))
		g.Ranks[name]++
		g.mu.Unlock()
	}
}

func main() {
	g := &Guild{
		Board:  make(chan Quest, 16),
		Heroes: []Hero{&Warrior{Name: "Борн"}, &Mage{Name: "Мирра", Mana: 100}},
		Ranks:  map[string]int{},
	}
	for i := int32(1); i <= 16; i++ {
		g.Board <- Quest{ID: i, Reward: int64(i) * 10, Title: fmt.Sprintf("заказ №%d", i)}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go worker(g, g.Heroes[0], "Борн", &wg)
	go worker(g, g.Heroes[1], "Мирра", &wg)

	// Странствие по ЖИВОЙ гильдии: работники продолжают трудиться.
	// Подвигай курсор через пару секунд — цифры уже другие.
	eye.Explore(g, "гильдия за работой")

	close(g.Board)
	wg.Wait()
	fmt.Printf("итог: золота %d, записей в журнале %d\n", g.Gold.Load(), len(g.Log))
}
