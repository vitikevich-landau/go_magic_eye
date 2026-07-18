package eye_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	eye "github.com/vitikevich-landau/go_magic_eye"
)

type loot struct {
	Gold int
	Gems []string
}

func TestFinspectWritesToWriter(t *testing.T) {
	var b strings.Builder
	l := loot{Gold: 1200, Gems: []string{"рубин"}}
	eye.Finspect(&b, &l, eye.WithLabel("казна"), eye.WithColor(false))
	out := b.String()
	if out == "" {
		t.Fatal("Finspect ничего не написал в писатель")
	}
	for _, want := range []string{"казна", "loot"} {
		if !strings.Contains(out, want) {
			t.Errorf("в выводе нет %q:\n%s", want, out)
		}
	}
}

func TestFinspectAutoPlain(t *testing.T) {
	t.Setenv("EYE_COLOR", "") // чистое окружение: решает автоматика
	var b strings.Builder
	eye.Finspect(&b, 42)
	if strings.Contains(b.String(), "\x1b[") {
		t.Error("не-терминальный писатель получил ANSI-коды, ожидался чистый текст")
	}
}

func TestOptionBeatsEnv(t *testing.T) {
	t.Setenv("EYE_COLOR", "1")
	var b strings.Builder
	eye.Finspect(&b, 42, eye.WithColor(false))
	if strings.Contains(b.String(), "\x1b[") {
		t.Error("WithColor(false) не перекрыл EYE_COLOR=1")
	}
}

func TestEnvColorForcesANSI(t *testing.T) {
	t.Setenv("EYE_COLOR", "1")
	var b strings.Builder
	eye.Finspect(&b, 42)
	if !strings.Contains(b.String(), "\x1b[") {
		t.Error("EYE_COLOR=1 не включил цвет для писателя-буфера")
	}
}

func TestWithWidthBeatsEnv(t *testing.T) {
	t.Setenv("EYE_WIDTH", "100")
	var b strings.Builder
	eye.Finspect(&b, &loot{Gold: 7},
		eye.WithWidth(60), eye.WithColor(false), eye.WithCenter(false))
	for _, line := range strings.Split(b.String(), "\n") {
		if utf8.RuneCountInString(line) > 60 {
			t.Errorf("строка шире WithWidth(60): %q", line)
		}
	}
}

func TestFinspectType(t *testing.T) {
	var b strings.Builder
	eye.FinspectType[loot](&b, eye.WithColor(false))
	if !strings.Contains(b.String(), "loot") {
		t.Errorf("в паспорте типа нет имени loot:\n%s", b.String())
	}
}

// Галерея с писателем-буфером печатает статикой все корни — и живой, и
// добавленный маркером типа через универсальный Add (диспетчеризация TypeMarker).
func TestGalleryStaticRun(t *testing.T) {
	var b strings.Builder
	l := loot{Gold: 7}
	g := eye.NewGallery(eye.WithWriter(&b), eye.WithColor(false))
	g.Add(&l, "сокровищница").Add(eye.TypeOf[Hero]())
	if err := g.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := b.String()
	for _, want := range []string{"сокровищница", "Hero"} {
		if !strings.Contains(out, want) {
			t.Errorf("в статической печати галереи нет %q", want)
		}
	}
}

func TestGalleryScriptRun(t *testing.T) {
	t.Setenv("EYE_SCRIPT", "down enter q")
	t.Setenv("EYE_COLOR", "0")
	var b strings.Builder
	l := loot{Gold: 7, Gems: []string{"рубин", "изумруд"}}
	g := eye.NewGallery(eye.WithWriter(&b), eye.WithWidth(100))
	g.Add(&l, "сокровищница")
	if err := g.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(b.String(), "кадр") {
		t.Errorf("EYE_SCRIPT-режим не напечатал кадры в писатель галереи:\n%.200s", b.String())
	}
}
