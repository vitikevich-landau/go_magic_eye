package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// libDir — корень библиотеки Ока: три уровня вверх от этого пакета.
func libDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.Abs(filepath.Join(wd, "..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "eye.go")); err != nil {
		t.Fatalf("корень библиотеки не найден в %s", dir)
	}
	return dir
}

func newRunner(t *testing.T) *Runner {
	return New(Options{LibDir: libDir(t)})
}

const okSnippet = `package main

import (
	"fmt"

	eye "github.com/vitikevich-landau/go_magic_eye"
)

type Hero struct {
	HP    int32
	Armor int8
}

func main() {
	h := Hero{HP: 100, Armor: 7}
	fmt.Println("до осмотра")
	eye.Inspect(&h, "герой")
	fmt.Println("после осмотра")
}
`

// Полный счастливый путь: компиляция, запуск, конверт извлечён, печать
// пользователя уцелела. Живой go toolchain обязателен (есть и в CI).
func TestRunHappyPath(t *testing.T) {
	res, err := newRunner(t).Run(context.Background(), okSnippet)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.OK || res.TimedOut {
		t.Fatalf("прогон не удался: %+v (stderr: %s)", res, res.Stderr)
	}
	if res.Envelope == nil {
		t.Fatalf("конверт Ока не извлечён; stdout: %q, stderr: %q", res.Stdout, res.Stderr)
	}
	if !strings.Contains(string(res.Envelope), "\"label\":\"герой\"") &&
		!strings.Contains(string(res.Envelope), "\"label\": \"герой\"") {
		t.Errorf("в конверте нет подписи: %.300s", res.Envelope)
	}
	for _, want := range []string{"до осмотра", "после осмотра"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("печать пользователя потеряна (%q): %q", want, res.Stdout)
		}
	}
}

func TestCheckReportsPositions(t *testing.T) {
	bad := "package main\n\nfunc main() {\n\tневедомая()\n}\n"
	res, err := newRunner(t).Check(context.Background(), bad)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.OK || len(res.Diags) == 0 {
		t.Fatalf("ошибка компиляции не поймана: %+v", res)
	}
	if res.Diags[0].Line != 4 {
		t.Errorf("позиция ошибки: %+v, ожидалась строка 4", res.Diags[0])
	}
}

// Бесконечный цикл убивается по таймауту, а не вешает песочницу.
func TestRunKillsInfiniteLoop(t *testing.T) {
	r := New(Options{LibDir: libDir(t), RunTimeout: 500 * time.Millisecond})
	t0 := time.Now()
	res, err := r.Run(context.Background(), "package main\n\nfunc main() { for {} }\n")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.TimedOut {
		t.Fatal("вечный цикл не помечен TimedOut")
	}
	if took := time.Since(t0); took > 15*time.Second {
		t.Errorf("убийство заняло %s", took)
	}
}

// Сторонний модуль не собирается: GOPROXY=off отрезает мир.
func TestRunForeignImportRejected(t *testing.T) {
	code := "package main\n\nimport \"github.com/fatih/color\"\n\nfunc main() { color.Red(\"нет\") }\n"
	res, err := newRunner(t).Run(context.Background(), code)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.OK {
		t.Fatal("чужой модуль внезапно собрался")
	}
}

func TestOversizeSnippetRejected(t *testing.T) {
	r := New(Options{LibDir: libDir(t), MaxCode: 64})
	if _, err := r.Run(context.Background(), strings.Repeat("//x\n", 100)); err == nil {
		t.Fatal("великан-снипетт прошёл лимит")
	}
}
