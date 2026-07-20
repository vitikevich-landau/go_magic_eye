package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// fmt.Print без \n прямо перед Inspect: конверт всё равно извлекается,
// печать остаётся в stdout (ревью Codex, третий заход).
func TestRunUnterminatedPrintBeforeInspect(t *testing.T) {
	code := "package main\n\nimport (\n\t\"fmt\"\n\n\teye \"github.com/vitikevich-landau/go_magic_eye\"\n)\n\nfunc main() {\n\tx := 42\n\tfmt.Print(\"progress: \")\n\teye.Inspect(&x, \"ответ\")\n}\n"
	res, err := newRunner(t).Run(context.Background(), code)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Envelope == nil {
		t.Fatalf("конверт не извлечён; stdout: %q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "progress:") {
		t.Errorf("незавершённая печать пропала: %q", res.Stdout)
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

// «package foo» — частая опечатка: go build успешно пишет АРХИВ, не бинарь,
// и раньше запуск падал сбоем песочницы (500). Теперь — диагностика с
// позицией, единая для check/run/explore.
func TestRunNotMainPackage(t *testing.T) {
	r := newRunner(t)
	code := "package foo\n\nfunc main() {}\n"

	check, err := r.Check(context.Background(), code)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if check.OK || len(check.Diags) == 0 {
		t.Fatalf("check пропустил package foo: %+v", check)
	}

	res, err := r.Run(context.Background(), code)
	if err != nil {
		t.Fatalf("Run вернул сбой песочницы вместо диагностики: %v", err)
	}
	if res.OK || len(res.Diags) == 0 {
		t.Fatalf("run пропустил package foo: %+v", res)
	}
	d := res.Diags[0]
	if d.Line != 1 || !strings.Contains(d.Message, "package main") {
		t.Errorf("диагностика не о package main или без позиции: %+v", d)
	}
}

// Путь библиотеки с ПРОБЕЛОМ не валит go.mod снипетта: replace в кавычках.
func TestRunLibDirWithSpaces(t *testing.T) {
	spaced := filepath.Join(t.TempDir(), "magic eye")
	if err := os.Symlink(libDir(t), spaced); err != nil {
		t.Skipf("символьная ссылка не создалась: %v", err)
	}
	res, err := New(Options{LibDir: spaced}).Run(context.Background(), okSnippet)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.OK {
		t.Fatalf("снипетт не собрался с пробельным LibDir: %+v (stderr: %s)", res.Diags, res.Stderr)
	}
	if res.Envelope == nil {
		t.Fatal("конверт не извлечён")
	}
}

// Обжора памяти умирает об жёсткий потолок (RLIMIT_AS), не съев хост:
// GOMEMLIMIT — лишь цель GC, настоящий заслон — prlimit (linux-only).
func TestRunMemoryHogHitsHardLimit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("prlimit на чужой процесс есть только на Linux")
	}
	code := `package main

import "fmt"

func main() {
	hog := make([][]byte, 0)
	for i := 0; i < 4096; i++ { // 4 ГиБ с касанием страниц
		b := make([]byte, 1<<20)
		for j := 0; j < len(b); j += 4096 {
			b[j] = 1
		}
		hog = append(hog, b)
	}
	fmt.Println(len(hog)) // не должно напечататься
}
`
	r := New(Options{LibDir: libDir(t), RunTimeout: 15 * time.Second})
	res, err := r.Run(context.Background(), code)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.TimedOut {
		t.Fatal("обжора дожил до таймаута — жёсткий потолок памяти не сработал")
	}
	if res.ExitCode == 0 {
		t.Fatalf("обжора вышел чисто (stdout: %q) — потолок не сработал", res.Stdout)
	}
}

// Фоновый ребёнок снипетта (exec…Start()) не переживает нормальный выход:
// группа процессов добивается и без таймаута.
func TestRunReapsBackgroundChildren(t *testing.T) {
	requirePgrep(t)
	const magic = "31337"
	code := "package main\n\nimport (\n\t\"os/exec\"\n\n\teye \"github.com/vitikevich-landau/go_magic_eye\"\n)\n\nfunc main() {\n\texec.Command(\"sleep\", \"" + magic + "\").Start()\n\tx := 1\n\teye.Inspect(&x)\n}\n"
	res, err := newRunner(t).Run(context.Background(), code)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.OK {
		t.Fatalf("прогон не удался: stderr %q", res.Stderr)
	}
	assertNoProcess(t, "sleep "+magic)
}

func requirePgrep(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pgrep"); err != nil {
		t.Skip("pgrep недоступен — проверку утечки детей пропускаем")
	}
}

// assertNoProcess — процесса с такой командной строкой не осталось
// (SIGKILL группе доставляется мгновенно, но даём планировщику вздохнуть).
func assertNoProcess(t *testing.T, pattern string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		if err := exec.Command("pgrep", "-f", pattern).Run(); err != nil {
			return // не найден — утечки нет
		}
		select {
		case <-deadline:
			t.Fatalf("фоновый процесс %q пережил уборку", pattern)
		case <-time.After(100 * time.Millisecond):
		}
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
