package diag

import (
	"strings"
	"testing"
)

func TestParsePositions(t *testing.T) {
	out := "# snippet\n" +
		"./main.go:12:7: undefined: knight\n" +
		"./main.go:20:2: cannot use \"x\" (untyped string constant) as int value\n"
	d := Parse(out)
	if len(d) != 2 {
		t.Fatalf("диагностик %d, ожидалось 2: %+v", len(d), d)
	}
	if d[0].Line != 12 || d[0].Col != 7 || d[0].Message != "undefined: knight" {
		t.Errorf("первая диагностика: %+v", d[0])
	}
	if d[0].Severity != "error" {
		t.Errorf("severity = %q", d[0].Severity)
	}
}

func TestParseNoColumn(t *testing.T) {
	d := Parse("main.go:3: некая беда\n")
	if len(d) != 1 || d[0].Line != 3 || d[0].Col != 1 {
		t.Fatalf("ошибка без колонки: %+v", d)
	}
}

func TestParseContinuationGluedToPrevious(t *testing.T) {
	out := "./main.go:9:10: cannot use k (variable of type *Knight) as Greeter value\n" +
		"\thave Hail() int\n" +
		"\twant Hail() string\n"
	d := Parse(out)
	if len(d) != 1 {
		t.Fatalf("диагностик %d, ожидалась 1: %+v", len(d), d)
	}
	if want := "have Hail() int"; !strings.Contains(d[0].Message, want) {
		t.Errorf("продолжение не приклеилось: %q", d[0].Message)
	}
}

func TestParseGarbageIgnored(t *testing.T) {
	if d := Parse("что-то пошло не так\nsegfault\n"); len(d) != 0 {
		t.Errorf("мусор превратился в диагностики: %+v", d)
	}
}
