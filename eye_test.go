package eye_test

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
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

// EYE_COLOR задан явно — он сильнее общего NO_COLOR (конкретное бьёт общее).
func TestEnvColorBeatsNoColor(t *testing.T) {
	t.Setenv("EYE_COLOR", "1")
	t.Setenv("NO_COLOR", "1")
	var b strings.Builder
	eye.Finspect(&b, 42)
	if !strings.Contains(b.String(), "\x1b[") {
		t.Error("явный EYE_COLOR=1 должен перекрывать NO_COLOR")
	}
}

// Значение в выноске переносится, а не обрезается рамкой: хвост («len 14»)
// обязан дожить до вывода даже на узком экране.
func TestNarrowWidthKeepsValueTail(t *testing.T) {
	type banner struct{ Motto string }
	bn := banner{Motto: "Semper fidelis"}
	var b strings.Builder
	eye.Finspect(&b, &bn, eye.WithWidth(60), eye.WithColor(false), eye.WithCenter(false))
	if !strings.Contains(b.String(), "len 14") {
		t.Errorf("хвост значения строки потерян на узкой ширине:\n%s", b.String())
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

// jsonEnvelope — распарсенный конверт машинного вида (для тестов ниже).
func jsonEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("вывод не парсится как JSON: %v\n%.300s", err, out)
	}
	if v, ok := env["eye_json_version"].(float64); !ok || v != 1 {
		t.Fatalf("eye_json_version = %v, ожидалась 1", env["eye_json_version"])
	}
	return env
}

func TestFinspectJSONFormat(t *testing.T) {
	var b strings.Builder
	l := loot{Gold: 1200, Gems: []string{"рубин"}}
	eye.Finspect(&b, &l, eye.WithLabel("казна"), eye.WithFormat(eye.JSON))
	env := jsonEnvelope(t, b.String())
	m := env["models"].([]any)[0].(map[string]any)
	if m["label"] != "казна" {
		t.Errorf("label = %v", m["label"])
	}
	if strings.Contains(b.String(), "\x1b[") {
		t.Error("в JSON-виде оказались ANSI-коды")
	}
}

func TestEnvFormatJSON(t *testing.T) {
	t.Setenv("EYE_FORMAT", "json")
	var b strings.Builder
	eye.Finspect(&b, 42)
	jsonEnvelope(t, b.String())
}

// EYE_JSON_FD уводит конверт в отдельный дескриптор: писателю (stdout
// программы) не достаётся ничего — машинный канал отделён от человеческого.
func TestEnvJSONFDRedirect(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	t.Setenv("EYE_FORMAT", "json")
	t.Setenv("EYE_JSON_FD", fmt.Sprint(w.Fd()))

	var b strings.Builder
	eye.Finspect(&b, 42, eye.WithLabel("ответ"))

	if b.String() != "" {
		t.Errorf("конверт просочился в писатель: %.120q", b.String())
	}
	buf := make([]byte, 64*1024)
	n, err := r.Read(buf)
	if err != nil || n == 0 {
		t.Fatalf("в канале конвертов пусто: %v", err)
	}
	jsonEnvelope(t, strings.TrimSpace(string(buf[:n])))
}

// Дескриптор конвертов переживает GC: одноразовая обёртка os.NewFile
// после сборки мусора закрыла бы fd финализатором, и второй Inspect молча
// падал бы в фолбэк — кэш jsonFDs держит обёртку живой.
func TestEnvJSONFDSurvivesGC(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	t.Setenv("EYE_FORMAT", "json")
	t.Setenv("EYE_JSON_FD", fmt.Sprint(w.Fd()))

	var b strings.Builder
	eye.Finspect(&b, 1, eye.WithLabel("первый"))
	runtime.GC()
	runtime.GC() // финализаторы отрабатывают со второго круга
	eye.Finspect(&b, 2, eye.WithLabel("второй"))

	if b.String() != "" {
		t.Fatalf("после GC конверт упал в фолбэк: %.120q", b.String())
	}
	got := ""
	buf := make([]byte, 64*1024)
	deadline := time.Now().Add(3 * time.Second)
	for strings.Count(got, "eye_json_version") < 2 {
		r.SetReadDeadline(deadline)
		n, rerr := r.Read(buf)
		got += string(buf[:n])
		if rerr != nil {
			t.Fatalf("дочитать оба конверта не вышло: %v\nполучено: %.200q", rerr, got)
		}
	}
}

// Конкурентные Inspect'ы не плодят обёрток одного fd: гонка LoadOrStore
// дала бы вторую обёртку, чей финализатор закрыл бы общий дескриптор.
// Под go test -race тест ловит и гонки данных на кэше.
func TestEnvJSONFDConcurrentInspects(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	t.Setenv("EYE_FORMAT", "json")
	t.Setenv("EYE_JSON_FD", fmt.Sprint(w.Fd()))

	const n = 8
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var b strings.Builder
			eye.Finspect(&b, i)
			if b.String() != "" {
				t.Errorf("конверт упал в фолбэк: %.80q", b.String())
			}
		}()
	}
	wg.Wait()
	runtime.GC()
	runtime.GC()

	got := ""
	buf := make([]byte, 256*1024)
	deadline := time.Now().Add(3 * time.Second)
	for strings.Count(got, "eye_json_version") < n {
		r.SetReadDeadline(deadline)
		nn, rerr := r.Read(buf)
		got += string(buf[:nn])
		if rerr != nil {
			t.Fatalf("дошло %d конвертов из %d: %v", strings.Count(got, "eye_json_version"), n, rerr)
		}
	}
}

func TestWithFormatBeatsEnv(t *testing.T) {
	t.Setenv("EYE_FORMAT", "json")
	var b strings.Builder
	eye.Finspect(&b, 42, eye.WithFormat(eye.Text), eye.WithColor(false))
	if strings.HasPrefix(strings.TrimSpace(b.String()), "{") {
		t.Error("WithFormat(Text) не перекрыл EYE_FORMAT=json")
	}
}

// Галерея в JSON-режиме отдаёт ОДИН конверт со всеми корнями — даже при
// завалявшемся EYE_SCRIPT (машинный вид сильнее скрипта).
func TestGalleryJSONRun(t *testing.T) {
	t.Setenv("EYE_SCRIPT", "down q")
	var b strings.Builder
	l := loot{Gold: 7}
	g := eye.NewGallery(eye.WithWriter(&b), eye.WithFormat(eye.JSON))
	g.Add(&l, "сокровищница").Add(eye.TypeOf[Hero]())
	if err := g.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	env := jsonEnvelope(t, b.String())
	ms := env["models"].([]any)
	if len(ms) != 2 {
		t.Fatalf("моделей %d, ожидалось 2", len(ms))
	}
	if ms[0].(map[string]any)["label"] != "сокровищница" {
		t.Errorf("label первого корня = %v", ms[0].(map[string]any)["label"])
	}
	if ms[1].(map[string]any)["has_value"] != false {
		t.Error("корень-тип (TypeOf) обязан прийти с has_value=false")
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
