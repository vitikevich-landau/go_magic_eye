package tui

import (
	"os"
	"path/filepath"
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
	for _, want := range []string{"странствие Ока", "странник", "память", "Tab ", "▌ ДЕРЕВО ▐"} {
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

// Снимок — документ, а не дамп экрана: без ANSI, без хвостовых пробелов,
// не шире snapWidth, с деревом и деталями.
func TestSnapshotFileFormat(t *testing.T) {
	dir := t.TempDir()
	app := NewApp(session(t), dir)
	app.W, app.H = 100, 24
	for _, tok := range []string{"enter", "down", "s"} {
		k, _ := ParseScriptKey(tok)
		app.Handle(k)
	}
	data, err := os.ReadFile(filepath.Join(dir, "eye_snap_001.txt"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, want := range []string{"странствие Ока", "── дерево", "── детали: Name", "странник", "Bag"} {
		if !strings.Contains(out, want) {
			t.Fatalf("в снимке нет %q:\n%s", want, out)
		}
	}
	for i, l := range strings.Split(out, "\n") {
		if strings.ContainsRune(l, 0x1b) {
			t.Fatalf("ANSI в снимке (строка %d): %q", i, l)
		}
		if strings.TrimRight(l, " ") != l {
			t.Fatalf("хвостовые пробелы (строка %d): %q", i, l)
		}
		if w := text.VisWidth(l); w > snapWidth {
			t.Fatalf("строка %d шире %d (%d): %q", i, snapWidth, w, l)
		}
	}
	// второй снимок не затирает первый
	k, _ := ParseScriptKey("s")
	app.Handle(k)
	if _, err := os.Stat(filepath.Join(dir, "eye_snap_002.txt")); err != nil {
		t.Fatal("второй снимок должен получить следующий номер:", err)
	}
}

// Фокус: Tab переключает зоны с явной плашкой-ярлыком; ← из деталей
// возвращает в дерево, НЕ сворачивая выбранный узел.
func TestFocusTabAndLeft(t *testing.T) {
	app := NewApp(session(t), t.TempDir())
	app.W, app.H = 100, 24

	frame := strings.Join(app.Frame(), "\n")
	if !strings.Contains(frame, "▌ ДЕРЕВО ▐") || !strings.Contains(frame, "── детали") {
		t.Fatalf("стартовые ярлыки зон не те:\n%s", frame)
	}

	enter, _ := ParseScriptKey("enter")
	tab, _ := ParseScriptKey("tab")
	left, _ := ParseScriptKey("left")

	app.Handle(enter) // раскрыть корень
	root := app.S.Roots[0]
	if !root.Expanded {
		t.Fatal("корень не раскрылся")
	}
	app.Handle(tab)
	frame = strings.Join(app.Frame(), "\n")
	if !strings.Contains(frame, "▌ ДЕТАЛИ ▐") || !strings.Contains(frame, "── дерево") {
		t.Fatalf("после Tab активной должна стать плашка ДЕТАЛИ:\n%s", frame)
	}
	if !strings.Contains(frame, "[детали]") {
		t.Fatal("гид внизу не показывает фокус [детали]")
	}

	app.Handle(left) // из деталей — назад в дерево, узел НЕ трогаем
	if !root.Expanded {
		t.Fatal("← из зоны деталей свернул узел — а должен только вернуть фокус")
	}
	frame = strings.Join(app.Frame(), "\n")
	if !strings.Contains(frame, "▌ ДЕРЕВО ▐") {
		t.Fatal("← из деталей не вернул фокус в дерево")
	}

	app.Handle(left) // а теперь фокус в дереве — узел сворачивается
	if root.Expanded {
		t.Fatal("← в дереве должен сворачивать узел")
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
