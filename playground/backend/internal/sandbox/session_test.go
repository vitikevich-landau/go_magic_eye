package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

const exploreSnippet = `package main

import (
	"fmt"

	eye "github.com/vitikevich-landau/go_magic_eye"
)

type Guild struct {
	Name   string
	Leader *Knight
}

type Knight struct {
	HP   int32
	Home *Guild
}

func main() {
	g := &Guild{Name: "Орден"}
	k := &Knight{HP: 100, Home: g}
	g.Leader = k
	fmt.Println("подъём флагов") // печать до странствия — не ломает протокол
	eye.Explore(g, "гильдия")
}
`

func startSession(t *testing.T, r *Runner) *Live {
	t.Helper()
	live, res, err := r.StartSession(context.Background(), exploreSnippet)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if !res.OK || live == nil {
		t.Fatalf("сеанс не начался: %+v", res)
	}
	t.Cleanup(live.Close)
	return live
}

func TestSessionLifecycle(t *testing.T) {
	r := newRunner(t)
	live := startSession(t, r)

	// hello: корни на месте
	var roots []map[string]any
	if err := json.Unmarshal(live.Roots, &roots); err != nil {
		t.Fatalf("roots не парсятся: %v\n%s", err, live.Roots)
	}
	if len(roots) != 1 || roots[0]["label"] != "гильдия" {
		t.Fatalf("roots: %v", roots)
	}
	rootID := int(roots[0]["id"].(float64))

	// печать до странствия дошла как noise
	if noise := live.Noise(); !strings.Contains(noise, "подъём флагов") {
		t.Errorf("печать пользователя потеряна: %q", noise)
	}

	// kids корня
	raw, err := live.Do("kids", rootID)
	if err != nil {
		t.Fatalf("kids: %v", err)
	}
	var kidsResp struct {
		OK    bool `json:"ok"`
		Nodes []struct {
			ID    int    `json:"id"`
			Label string `json:"label"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &kidsResp); err != nil || !kidsResp.OK {
		t.Fatalf("ответ kids: %s (%v)", raw, err)
	}
	if len(kidsResp.Nodes) != 2 {
		t.Fatalf("детей %d, ожидалось 2: %s", len(kidsResp.Nodes), raw)
	}

	// detail корня — конверт JSON-вида
	raw, err = live.Do("detail", rootID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	var detResp struct {
		OK  bool `json:"ok"`
		Eye struct {
			Version int `json:"eye_json_version"`
		} `json:"eye"`
	}
	if err := json.Unmarshal(raw, &detResp); err != nil || !detResp.OK || detResp.Eye.Version != 1 {
		t.Fatalf("ответ detail: %.200s (%v)", raw, err)
	}

	// закрытие: процесс умирает, сеанс исчезает из реестра
	id := live.ID
	live.Close()
	if r.Session(id) != nil {
		t.Error("сеанс остался в реестре после Close")
	}
	if _, err := live.Do("kids", rootID); err == nil {
		t.Error("команда мёртвому сеансу прошла без ошибки")
	}
}

// Снипетт без Explore — честный ErrNoSession, не зависание.
func TestSessionNoExplore(t *testing.T) {
	r := newRunner(t)
	code := "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"и всё\") }\n"
	live, _, err := r.StartSession(context.Background(), code)
	if err == nil {
		live.Close()
		t.Fatal("сеанс без Explore внезапно начался")
	}
	if !errors.Is(err, ErrNoSession) {
		t.Errorf("отказ не класса ErrNoSession: %v", err)
	}
}

// Программа копается дольше HelloWait перед Explore — это вина снипетта,
// не песочницы: отказ обязан быть класса ErrNoSession (API ответит
// пользовательской ошибкой, а не 500).
func TestSessionHelloTimeoutIsUserError(t *testing.T) {
	r := New(Options{LibDir: libDir(t), HelloWait: 300 * time.Millisecond})
	code := `package main

import (
	"time"

	eye "github.com/vitikevich-landau/go_magic_eye"
)

func main() {
	time.Sleep(5 * time.Second) // «долгая подготовка»
	x := 1
	eye.Explore(&x)
}
`
	live, _, err := r.StartSession(context.Background(), code)
	if err == nil {
		live.Close()
		t.Fatal("сеанс начался вопреки HelloWait")
	}
	if !errors.Is(err, ErrNoSession) {
		t.Errorf("таймаут рукопожатия не класса ErrNoSession: %v", err)
	}
}

// Жнец закрывает простаивающий сеанс.
func TestSessionReaper(t *testing.T) {
	r := New(Options{
		LibDir:      libDir(t),
		SessionIdle: 300 * time.Millisecond,
		ReapTick:    100 * time.Millisecond,
	})
	live := startSession(t, r)
	id := live.ID
	deadline := time.After(5 * time.Second)
	for r.Session(id) != nil {
		select {
		case <-deadline:
			t.Fatal("жнец не закрыл простаивающий сеанс")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Потолок числа сеансов держится.
func TestSessionMax(t *testing.T) {
	r := New(Options{LibDir: libDir(t), SessionMax: 1})
	_ = startSession(t, r)
	if live2, _, err := r.StartSession(context.Background(), exploreSnippet); err == nil {
		live2.Close()
		t.Fatal("второй сеанс прошёл сквозь SessionMax=1")
	}
}

// …и держится под одновременным штурмом: бронь слота атомарна, пачка
// параллельных explore не пробивает SessionMax (TOCTOU-гонка из ревью).
func TestSessionMaxConcurrent(t *testing.T) {
	r := New(Options{LibDir: libDir(t), SessionMax: 1})
	const n = 4
	results := make(chan *Live, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			live, res, err := r.StartSession(context.Background(), exploreSnippet)
			if err != nil {
				errs <- err
				return
			}
			if live == nil {
				errs <- fmt.Errorf("нет ошибки, но нет и сеанса: %+v", res)
				return
			}
			results <- live
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	var started []*Live
	for live := range results {
		started = append(started, live)
	}
	for _, live := range started {
		defer live.Close()
	}
	if len(started) != 1 {
		var all []string
		for err := range errs {
			all = append(all, err.Error())
		}
		t.Fatalf("сквозь SessionMax=1 прошло %d сеансов; отказы: %s",
			len(started), strings.Join(all, " | "))
	}
}

// Печать БЕЗ завершающего \n прямо перед Explore не съедает рукопожатие:
// hello начинается с чистой строки (библиотека), а склейки расклеивает
// насос (splitProtocol).
func TestSessionHelloAfterUnterminatedPrint(t *testing.T) {
	code := `package main

import (
	"fmt"

	eye "github.com/vitikevich-landau/go_magic_eye"
)

func main() {
	x := 42
	fmt.Print("прогресс: ") // нарочно без \n
	eye.Explore(&x, "ответ")
}
`
	r := newRunner(t)
	live, res, err := r.StartSession(context.Background(), code)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	defer live.Close()
	if !res.OK {
		t.Fatalf("сеанс не начался: %+v", res)
	}
	if noise := live.Noise(); !strings.Contains(noise, "прогресс:") {
		t.Errorf("незавершённая печать потерялась: %q", noise)
	}
}

// Фоновый ребёнок, рождённый кодом странствия, не переживает чистый Close.
func TestSessionCloseReapsChildren(t *testing.T) {
	requirePgrep(t)
	const magic = "31338"
	code := `package main

import (
	"os/exec"

	eye "github.com/vitikevich-landau/go_magic_eye"
)

func main() {
	exec.Command("sleep", "` + magic + `").Start()
	x := 1
	eye.Explore(&x, "ответ")
}
`
	r := newRunner(t)
	live, res, err := r.StartSession(context.Background(), code)
	if err != nil || !res.OK {
		t.Fatalf("StartSession: %v / %+v", err, res)
	}
	live.Close()
	assertNoProcess(t, "sleep "+magic)
}

// Close посреди летящей команды не крадёт у неё ответ и не виснет:
// quit сериализован тем же мьютексом, id команды — локальная копия.
func TestCloseDuringInflightCommand(t *testing.T) {
	r := newRunner(t)
	live := startSession(t, r)
	var roots []map[string]any
	if err := json.Unmarshal(live.Roots, &roots); err != nil {
		t.Fatal(err)
	}
	rootID := int(roots[0]["id"].(float64))

	done := make(chan struct{})
	go func() {
		defer close(done)
		// любой исход законен (ответ или ErrSessionGone) — лишь бы не
		// зависнуть и не перепутать ответы
		if raw, err := live.Do("detail", rootID); err == nil {
			var resp struct {
				OK  bool            `json:"ok"`
				Eye json.RawMessage `json:"eye"`
			}
			if json.Unmarshal(raw, &resp) == nil && resp.OK && resp.Eye == nil {
				t.Error("Do принял чужой ответ (ok без eye — похоже на quit)")
			}
		}
	}()
	live.Close()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Do завис после конкурентного Close")
	}
}

// Поддельный hello (пользовательский лог с eye_session_version, но без
// roots) не открывает сеанс: настоящее рукопожатие обязано нести массив
// корней — сеанс стартует по нему, а не по логу.
func TestSessionFakeHelloIgnored(t *testing.T) {
	r := newRunner(t)
	code := `package main

import (
	"fmt"

	eye "github.com/vitikevich-landau/go_magic_eye"
)

func main() {
	fmt.Println(` + "`{\"eye_session_version\":1}`" + `) // лог-самозванец
	x := 42
	eye.Explore(&x, "настоящий")
}
`
	live, res, err := r.StartSession(context.Background(), code)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	defer live.Close()
	if !res.OK {
		t.Fatalf("сеанс не начался: %+v", res)
	}
	var roots []map[string]any
	if err := json.Unmarshal(live.Roots, &roots); err != nil || len(roots) != 1 {
		t.Fatalf("сеанс открыт не настоящим hello: %s (%v)", live.Roots, err)
	}
	if roots[0]["label"] != "настоящий" {
		t.Errorf("корень не от настоящего hello: %v", roots[0])
	}
	// лог-самозванец — печать пользователя: он обязан дойти до stdout,
	// а не пропасть между насосом и рукопожатием
	if noise := live.Noise(); !strings.Contains(noise, "eye_session_version") {
		t.Errorf("самозванец пропал из stdout: %q", noise)
	}
}

// Паника до Explore приходит с отказом НЕ голой: stderr с причиной
// сохраняется в RunResult — пользователь видит, почему сеанса нет.
func TestSessionNoHelloCarriesStderr(t *testing.T) {
	r := newRunner(t)
	code := "package main\n\nfunc main() { panic(\"упал до странствия\") }\n"
	live, res, err := r.StartSession(context.Background(), code)
	if err == nil {
		live.Close()
		t.Fatal("сеанс с паникой до Explore внезапно начался")
	}
	if !errors.Is(err, ErrNoSession) {
		t.Fatalf("отказ не класса ErrNoSession: %v", err)
	}
	if !strings.Contains(res.Stderr, "упал до странствия") {
		t.Errorf("stderr с причиной паники потерян: %q", res.Stderr)
	}
}

// Гигантская строка (длиннее MaxOutput) до странствия не убивает насос:
// хвост обрезается как шум, hello доходит, сеанс живёт.
func TestSessionSurvivesOversizedLine(t *testing.T) {
	r := New(Options{LibDir: libDir(t), MaxOutput: 64 * 1024}) // потолок поменьше — тест быстрый
	code := `package main

import (
	"fmt"
	"strings"

	eye "github.com/vitikevich-landau/go_magic_eye"
)

func main() {
	fmt.Println("до:" + strings.Repeat("x", 1<<20)) // строка на мегабайт
	x := 42
	eye.Explore(&x, "ответ")
}
`
	live, res, err := r.StartSession(context.Background(), code)
	if err != nil {
		t.Fatalf("StartSession: %v (гигантская строка убила насос?)", err)
	}
	defer live.Close()
	if !res.OK {
		t.Fatalf("сеанс не начался: %+v", res)
	}
	noise := live.Noise()
	if !strings.Contains(noise, "обрезана") {
		t.Errorf("обрезание гигантской строки не помечено: %.120q", noise)
	}
	// и протокол после неё жив
	var roots []map[string]any
	if err := json.Unmarshal(live.Roots, &roots); err != nil || len(roots) != 1 {
		t.Fatalf("roots после гигантской строки: %v / %s", err, live.Roots)
	}
	if _, err := live.Do("detail", int(roots[0]["id"].(float64))); err != nil {
		t.Errorf("detail после гигантской строки: %v", err)
	}
}

// Клиент отменил запрос (закрыл вкладку), пока снипетт готовился к
// странствию: сеанс не регистрируется и не живёт сиротой до жнеца.
func TestStartSessionClientCancelNotRegistered(t *testing.T) {
	r := newRunner(t)
	code := `package main

import (
	"time"

	eye "github.com/vitikevich-landau/go_magic_eye"
)

func main() {
	time.Sleep(5 * time.Second)
	x := 1
	eye.Explore(&x)
}
`
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(2 * time.Second) // компиляция успела, ожидание hello — нет
		cancel()
	}()
	live, _, err := r.StartSession(ctx, code)
	if err == nil {
		live.Close()
		t.Fatal("сеанс пережил отмену клиента")
	}
	r.sessMu.Lock()
	n := len(r.sessions)
	r.sessMu.Unlock()
	if n != 0 {
		t.Fatalf("после отмены клиента в реестре осталось %d сеансов", n)
	}
}

func TestSplitProtocol(t *testing.T) {
	for name, tc := range map[string]struct {
		in           string
		noise, proto string
	}{
		"чистый протокол":   {`{"id":1,"ok":true}`, "", `{"id":1,"ok":true}`},
		"чистый шум":        {"привет", "привет", ""},
		"склейка":           {`тик{"id":2,"ok":true}`, "тик", `{"id":2,"ok":true}`},
		"склейка hello":     {`x{"eye_session_version":1,"roots":[]}`, "x", `{"eye_session_version":1,"roots":[]}`},
		"json пользователя": {`{"my":"json"}`, `{"my":"json"}`, ""},
		// структурный лог с id, но без ok — НЕ протокол: не должен украсть
		// место ответа в Do
		"лог с id": {`{"id":1}`, `{"id":1}`, ""},
		// самозванец без корней — печать пользователя, не hello: обязан
		// дойти до stdout, а не пропасть между насосом и рукопожатием
		"самозванец hello": {`{"eye_session_version":1}`, `{"eye_session_version":1}`, ""},
	} {
		noise, proto := splitProtocol([]byte(tc.in))
		if string(noise) != tc.noise || string(proto) != tc.proto {
			t.Errorf("%s: noise=%q proto=%q, ожидалось %q/%q", name, noise, proto, tc.noise, tc.proto)
		}
	}
}
