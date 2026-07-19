package sandbox

import (
	"context"
	"encoding/json"
	"strings"
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
	if !strings.Contains(err.Error(), "Explore") {
		t.Errorf("причина отказа не про Explore: %v", err)
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
