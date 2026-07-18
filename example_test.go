package eye_test

import (
	"errors"
	"fmt"
	"os"
	"strings"

	eye "github.com/vitikevich-landau/go_magic_eye"
)

// Hero — учебный тип примеров: строка-заголовок, число с padding-дырой, срез.
type Hero struct {
	Name string
	HP   int32
	Bag  []string
}

func ExampleInspect() {
	h := Hero{Name: "Ланселот", HP: 100, Bag: []string{"меч", "эликсир"}}
	eye.Inspect(&h, "рыцарь") // по указателю — Око смотрит на оригинал
}

func ExampleInspectType() {
	eye.InspectType[Hero]("паспорт героя") // объект не нужен — только статика типа
}

func ExampleFinspect() {
	var b strings.Builder
	h := Hero{Name: "Галахад", HP: 90}
	eye.Finspect(&b, &h,
		eye.WithLabel("в буфер"),
		eye.WithColor(false), // чистый текст без ANSI
		eye.WithWidth(80),
	)
	fmt.Println(strings.Contains(b.String(), "в буфер"))
	// Output: true
}

func ExampleGallery() {
	g := eye.NewGallery()
	g.Add(&Hero{Name: "Ланселот"}, "рыцарь").AddType(eye.TypeOf[Hero]())
	if err := g.Run(); err != nil {
		if errors.Is(err, eye.ErrInterrupted) {
			os.Exit(130) // Ctrl-C: код выхода выбирает программа, не библиотека
		}
		fmt.Fprintln(os.Stderr, err)
	}
}

func ExampleExplore() {
	h := Hero{Name: "Персиваль", HP: 80}
	_ = eye.Explore(&h, "герой") // блокирует до выхода (q/Esc)
}
