package sandbox

import (
	"encoding/json"
	"strings"
	"testing"
)

const envA = "{\n  \"eye_json_version\": 1,\n  \"models\": [{\"label\": \"а\"}]\n}\n"
const envB = "{\n  \"eye_json_version\": 1,\n  \"models\": [{\"label\": \"б\"}, {\"label\": \"в\"}]\n}\n"

func modelCount(t *testing.T, envelope []byte) int {
	t.Helper()
	var e struct {
		Version int               `json:"eye_json_version"`
		Models  []json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(envelope, &e); err != nil {
		t.Fatalf("слитый конверт не парсится: %v\n%s", err, envelope)
	}
	if e.Version != 1 {
		t.Fatalf("версия слитого конверта = %d", e.Version)
	}
	return len(e.Models)
}

func TestExtractSingleEnvelope(t *testing.T) {
	env, rest := ExtractEnvelopes([]byte(envA))
	if env == nil {
		t.Fatal("конверт не найден")
	}
	if n := modelCount(t, env); n != 1 {
		t.Errorf("моделей %d, ожидалась 1", n)
	}
	if strings.TrimSpace(rest) != "" {
		t.Errorf("остаток не пуст: %q", rest)
	}
}

// Два Inspect'а + пользовательские fmt.Println между ними: конверты сливаются,
// печать пользователя остаётся в остатке в исходном порядке.
func TestExtractMixedOutput(t *testing.T) {
	mixed := "привет от пользователя\n" + envA + "между осмотрами\n" + envB + "и в конце\n"
	env, rest := ExtractEnvelopes([]byte(mixed))
	if n := modelCount(t, env); n != 3 {
		t.Errorf("моделей %d, ожидалось 3 (слияние двух конвертов)", n)
	}
	for _, want := range []string{"привет от пользователя", "между осмотрами", "и в конце"} {
		if !strings.Contains(rest, want) {
			t.Errorf("в остатке нет %q: %q", want, rest)
		}
	}
	if strings.Contains(rest, "eye_json_version") {
		t.Errorf("конверт просочился в остаток: %q", rest)
	}
}

// Пользовательский JSON без eye_json_version — не конверт: остаётся в stdout.
func TestExtractUserJSONNotStolen(t *testing.T) {
	user := "{\"my\": \"json\"}\n"
	env, rest := ExtractEnvelopes([]byte(user))
	if env != nil {
		t.Errorf("чужой JSON принят за конверт: %s", env)
	}
	if !strings.Contains(rest, "\"my\": \"json\"") {
		t.Errorf("пользовательский JSON пропал: %q", rest)
	}
}

// Два Inspect'а подряд без печати между ними — конверты стоят встык
// (именно так выглядит вывод обычного примера с двумя осмотрами).
func TestExtractConsecutiveEnvelopes(t *testing.T) {
	env, rest := ExtractEnvelopes([]byte(envA + envB))
	if n := modelCount(t, env); n != 3 {
		t.Errorf("моделей %d, ожидалось 3", n)
	}
	if strings.Contains(rest, "eye_json_version") {
		t.Errorf("конверт утёк в остаток: %q", rest)
	}
}

func TestExtractNoEnvelope(t *testing.T) {
	env, rest := ExtractEnvelopes([]byte("просто текст\n"))
	if env != nil || rest != "просто текст\n" {
		t.Errorf("env=%s rest=%q", env, rest)
	}
}
