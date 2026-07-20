package sandbox

import (
	"context"
	"encoding/json"
	"errors"
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
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			live, _, err := r.StartSession(context.Background(), exploreSnippet)
			if err == nil {
				results <- live
			}
		}()
	}
	wg.Wait()
	close(results)
	var started []*Live
	for live := range results {
		started = append(started, live)
	}
	for _, live := range started {
		defer live.Close()
	}
	if len(started) != 1 {
		t.Fatalf("сквозь SessionMax=1 прошло %d сеансов", len(started))
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
	} {
		noise, proto := splitProtocol([]byte(tc.in))
		if string(noise) != tc.noise || string(proto) != tc.proto {
			t.Errorf("%s: noise=%q proto=%q, ожидалось %q/%q", name, noise, proto, tc.noise, tc.proto)
		}
	}
}
