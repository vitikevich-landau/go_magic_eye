package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vitikevich-landau/go_magic_eye/playground/backend/internal/sandbox"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	wd, _ := os.Getwd()
	lib, _ := filepath.Abs(filepath.Join(wd, "..", "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(lib, "eye.go")); err != nil {
		t.Fatalf("корень библиотеки не найден в %s", lib)
	}
	srv := httptest.NewServer(New(sandbox.New(sandbox.Options{LibDir: lib}), nil))
	t.Cleanup(srv.Close)
	return srv
}

func post(t *testing.T, url, body string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("ответ не JSON: %v", err)
	}
	return resp, m
}

func TestCheckEndpointBadCode(t *testing.T) {
	srv := testServer(t)
	body, _ := json.Marshal(map[string]string{
		"code": "package main\n\nfunc main() {\n\tневедомая()\n}\n",
	})
	resp, m := post(t, srv.URL+"/api/check", string(body))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("статус %d", resp.StatusCode)
	}
	if m["ok"] != false {
		t.Error("ok=true у снипетта с ошибкой")
	}
	diags := m["diagnostics"].([]any)
	if len(diags) == 0 {
		t.Fatal("диагностики пусты")
	}
	d := diags[0].(map[string]any)
	if d["line"].(float64) != 4 {
		t.Errorf("line = %v", d["line"])
	}
}

func TestRunEndpointHappyPath(t *testing.T) {
	srv := testServer(t)
	code := "package main\n\nimport eye \"github.com/vitikevich-landau/go_magic_eye\"\n\nfunc main() {\n\tx := 42\n\teye.Inspect(&x, \"ответ\")\n}\n"
	body, _ := json.Marshal(map[string]string{"code": code})
	resp, m := post(t, srv.URL+"/api/run", string(body))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("статус %d: %v", resp.StatusCode, m)
	}
	if m["ok"] != true {
		t.Fatalf("ok=false: %v", m)
	}
	eye, isMap := m["eye"].(map[string]any)
	if !isMap {
		// диагностика на случай флейка под давлением памяти (пакеты тестов
		// бегут параллельно): чей это был сбой — виден по коду и stderr
		t.Fatalf("eye = %v, ожидался конверт (exit_code=%v, stderr=%q, stdout=%q)",
			m["eye"], m["exit_code"], m["stderr"], m["stdout"])
	}
	if eye["eye_json_version"].(float64) != 1 {
		t.Errorf("версия конверта: %v", eye["eye_json_version"])
	}
	if len(eye["models"].([]any)) != 1 {
		t.Errorf("моделей: %v", eye["models"])
	}
}

func TestBadRequests(t *testing.T) {
	srv := testServer(t)
	for name, tc := range map[string]struct {
		body string
		want int
	}{
		"не JSON":        {"это не json", http.StatusBadRequest},
		"пустой снипетт": {`{"code": ""}`, http.StatusBadRequest},
	} {
		resp, _ := post(t, srv.URL+"/api/check", tc.body)
		if resp.StatusCode != tc.want {
			t.Errorf("%s: статус %d, ожидался %d", name, resp.StatusCode, tc.want)
		}
	}
}

// Лимит меряется по декодированному коду: JSON-экранирование (\n → \\n)
// раздувает тело, и снипетт у самого потолка не должен ловить 413.
func TestCodeLimitOnDecodedCode(t *testing.T) {
	wd, _ := os.Getwd()
	lib, _ := filepath.Abs(filepath.Join(wd, "..", "..", "..", ".."))
	srv := httptest.NewServer(New(sandbox.New(sandbox.Options{LibDir: lib, MaxCode: 1024}), nil))
	t.Cleanup(srv.Close)

	pad := func(n int) string {
		head := "package main\n\nfunc main() {}\n"
		return head + strings.Repeat("// набивка\n", (n-len(head))/len("// набивка\n"))
	}
	// у потолка (много \n → тело JSON заметно больше 1024) — принят
	body, _ := json.Marshal(map[string]string{"code": pad(1000)})
	resp, _ := post(t, srv.URL+"/api/check", string(body))
	if resp.StatusCode == http.StatusRequestEntityTooLarge {
		t.Fatal("снипетт у потолка отвергнут 413 из-за JSON-обёртки")
	}
	// сверх потолка — честный 413
	body, _ = json.Marshal(map[string]string{"code": pad(4000)})
	resp, _ = post(t, srv.URL+"/api/check", string(body))
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("великан прошёл: статус %d", resp.StatusCode)
	}
}

func TestExamplesEndpoint(t *testing.T) {
	srv := testServer(t)
	resp, err := http.Get(srv.URL + "/api/examples")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) < 6 {
		t.Fatalf("примеров %d, ожидалось ≥ 6", len(list))
	}
	first := list[0]
	for _, key := range []string{"id", "title", "topic", "code"} {
		if first[key] == "" || first[key] == nil {
			t.Errorf("у примера пуст %q: %v", key, first)
		}
	}
	if strings.Contains(first["code"].(string), "//eye:") {
		t.Error("метаданные //eye: просочились в код примера")
	}
}
