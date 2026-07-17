package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/vitikevich-landau/go_magic_eye/internal/nav"
	"github.com/vitikevich-landau/go_magic_eye/internal/text"
)

// Снапшот-тесты странствия: как EYE_SCRIPT, но без подпроцесса — кадры
// рисуются в буфер, проверяются ключевые строки (не байт-в-байт: адреса
// и версия рантайма плавают).

type pouch struct {
	Gold int64
	Tags []string
}

type wanderer struct {
	Name string
	Bag  *pouch
	Pal  *wanderer
}

func session(t *testing.T) *nav.Session {
	t.Helper()
	w := &wanderer{Name: "Странник", Bag: &pouch{Gold: 7, Tags: []string{"x", "y"}}}
	w.Pal = w // цикл на себя
	s := nav.NewSession()
	s.AddRoot(reflect.ValueOf(w).Elem(), "странник")
	return s
}

func runScript(t *testing.T, tokens string) string {
	t.Helper()
	app := NewApp(session(t), t.TempDir())
	var b strings.Builder
	app.RunScript(strings.Fields(tokens), &b, 100, 24)
	return b.String()
}

func TestScriptStartFrame(t *testing.T) {
	out := runScript(t, "")
	for _, want := range []string{"странствие Ока", "странник", "память", "Tab фокус"} {
		if !strings.Contains(out, want) {
			t.Fatalf("в стартовом кадре нет %q\n%s", want, out)
		}
	}
}

func TestScriptExpandAndDetails(t *testing.T) {
	out := runScript(t, "enter down")
	for _, want := range []string{"Name", "Bag", "Pal", "Странник"} {
		if !strings.Contains(out, want) {
			t.Fatalf("после enter нет %q", want)
		}
	}
}

func TestScriptPointerFollow(t *testing.T) {
	out := runScript(t, "enter down down enter down enter")
	// Bag → ➤ цель pouch → поля Gold/Tags
	for _, want := range []string{"цель", "Gold", "Tags"} {
		if !strings.Contains(out, want) {
			t.Fatalf("переход по указателю потерял %q\n%s", want, out)
		}
	}
}

func TestScriptCycleMark(t *testing.T) {
	out := runScript(t, "enter down down down enter")
	if !strings.Contains(out, "уже показан") {
		t.Fatalf("цикл ⟲ не распознан:\n%s", out)
	}
}

func TestScriptPanelsAndHelp(t *testing.T) {
	out := runScript(t, "x")
	if !strings.Contains(out, "панель: hex") {
		t.Fatal("панель hex не включилась")
	}
	out = runScript(t, "?")
	if !strings.Contains(out, "свиток помощи") {
		t.Fatal("помощь не открылась")
	}
}

func TestScriptSearch(t *testing.T) {
	out := runScript(t, "enter / B a g enter")
	if !strings.Contains(out, "найдено: Bag") {
		t.Fatalf("поиск Bag не сработал:\n%s", out)
	}
}

func TestScriptQuit(t *testing.T) {
	out := runScript(t, "q")
	if !strings.Contains(out, "выход по «q»") {
		t.Fatal("q не вышел")
	}
}

// Ни одна строка кадра не смеет быть шире экрана: перелив вызывает
// автоперенос, прокрутку и мерцание всего терминала (регресс-тест).
func TestFrameLinesNeverOverflowWidth(t *testing.T) {
	app := NewApp(session(t), t.TempDir())
	app.W, app.H = 100, 24
	script := []string{"enter", "down", "enter", "down", "down", "enter",
		"tab", "pgdn", "x", "m", "?"}
	check := func(when string) {
		for i, l := range app.Frame() {
			if w := text.VisWidth(l); w > app.W {
				t.Fatalf("строка %d шире экрана (%d > %d) после %q:\n%q",
					i, w, app.W, when, l)
			}
		}
	}
	check("старт")
	for _, tok := range script {
		k, _ := ParseScriptKey(tok)
		app.Handle(k)
		check(tok)
	}
}
