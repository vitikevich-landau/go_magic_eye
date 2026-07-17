package text

import (
	"strings"
	"testing"
)

func TestVisWidthIgnoresANSI(t *testing.T) {
	plain := "привет, eye"
	colored := CName + "привет" + CReset + ", " + CVal + "eye" + CReset
	if VisWidth(plain) != VisWidth(colored) {
		t.Fatalf("ANSI не должен влиять на ширину: %d vs %d",
			VisWidth(plain), VisWidth(colored))
	}
	if VisWidth("Око") != 3 {
		t.Fatalf("кириллица ширины 1: %d", VisWidth("Око"))
	}
	if VisWidth("漢字") != 4 {
		t.Fatalf("CJK ширины 2: %d", VisWidth("漢字"))
	}
}

func TestClipVis(t *testing.T) {
	s := "длинная строка про Око мага"
	c := ClipVis(s, 10)
	if VisWidth(c) > 10 {
		t.Fatalf("обрезка шире лимита: %d %q", VisWidth(c), c)
	}
	if !strings.HasSuffix(c, "…") {
		t.Fatalf("нет многоточия: %q", c)
	}
	if got := ClipVis("короткая", 20); got != "короткая" {
		t.Fatalf("короткое не должно меняться: %q", got)
	}
	// цветная строка: escape-последовательности не рвутся
	colored := CWarn + "опасность повсюду вокруг нас" + CReset
	cc := ClipVis(colored, 12)
	if VisWidth(cc) > 12 {
		t.Fatalf("цветная обрезка шире лимита: %d", VisWidth(cc))
	}
}

func TestLineBuilder(t *testing.T) {
	old := Color
	Color = true
	defer func() { Color = old }()
	l := &Line{}
	l.Add(CName, "поле").Sp(2).Add(CVal, "42")
	if l.W() != 4+2+2 {
		t.Fatalf("ширина строителя: %d", l.W())
	}
	l.PadTo(20)
	if l.W() != 20 {
		t.Fatalf("PadTo: %d", l.W())
	}
	if VisWidth(l.String()) != 20 {
		t.Fatalf("итоговая видимая ширина: %d", VisWidth(l.String()))
	}
}

func TestPaintRespectsToggle(t *testing.T) {
	old := Color
	defer func() { Color = old }()
	Color = false
	if Paint(CWarn, "x") != "x" {
		t.Fatal("без цвета строка должна быть голой")
	}
	Color = true
	if !strings.Contains(Paint(CWarn, "x"), "\x1b[") {
		t.Fatal("с цветом должен быть ANSI")
	}
}
