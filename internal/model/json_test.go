package model

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// Контрактные тесты JSON-вида (playground/SPEC.md §2.1). Живые адреса
// недетерминированы — сравниваются структурные свойства и нормализованные
// значения, а не байты вывода целиком.

// gauge — без указателей: байты детерминированы (little-endian на всех
// 64-битных платформах Ока), а дыра выравнивания гарантирована.
type gauge struct {
	HP    int32   // 0..4
	Armor int8    // 4..5 → дыра 5..8 (float64 требует кратный 8)
	Speed float64 // 8..16
}

func envelopeOf(t *testing.T, models []*Model) map[string]any {
	t.Helper()
	b, err := ToJSON(models)
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("конверт не парсится обратно: %v\n%s", err, b)
	}
	return env
}

func TestJSONEnvelopeShape(t *testing.T) {
	g := gauge{HP: 100, Armor: 7, Speed: 1.5}
	env := envelopeOf(t, []*Model{Of(&g, "датчик")})

	if v, ok := env["eye_json_version"].(float64); !ok || int(v) != JSONVersion {
		t.Errorf("eye_json_version = %v, ожидалась %d", env["eye_json_version"], JSONVersion)
	}
	ms := env["models"].([]any)
	if len(ms) != 1 {
		t.Fatalf("моделей %d, ожидалась 1", len(ms))
	}
	m := ms[0].(map[string]any)

	if m["label"] != "датчик" {
		t.Errorf("label = %v", m["label"])
	}
	p := m["passport"].(map[string]any)
	if !strings.HasSuffix(p["type_name"].(string), "gauge") {
		t.Errorf("type_name = %v", p["type_name"])
	}
	// kind — человеческое слово («структура»), как и всюду в модели: его
	// показывают, по нему не ветвятся
	if p["kind"] != "структура" || p["size"].(float64) != 16 || p["align"].(float64) != 8 {
		t.Errorf("паспорт: kind=%v size=%v align=%v", p["kind"], p["size"], p["align"])
	}

	if m["has_value"] != true {
		t.Error("has_value = false у живого объекта")
	}
	if ok, _ := regexp.MatchString(`^0x[0-9a-f]+$`, m["addr"].(string)); !ok {
		t.Errorf("addr %q — не строка вида 0x…", m["addr"])
	}
	// 16 байт объекта → 32 hex-символа
	if b := m["bytes"].(string); len(b) != 32 {
		t.Errorf("bytes: %d hex-символов, ожидалось 32 (%q)", len(b), b)
	}
}

// Карта регионов детерминирована полностью — сравниваем как golden-таблицу.
func TestJSONRegionsGolden(t *testing.T) {
	g := gauge{HP: 100, Armor: 7, Speed: 1.5}
	env := envelopeOf(t, []*Model{Of(&g, "")})
	regions := env["models"].([]any)[0].(map[string]any)["regions"].([]any)

	want := []struct {
		kind   string
		offset float64
		size   float64
		name   string
	}{
		{"field", 0, 4, "HP"},
		{"field", 4, 1, "Armor"},
		{"padding", 5, 3, ""},
		{"field", 8, 8, "Speed"},
	}
	if len(regions) != len(want) {
		t.Fatalf("регионов %d, ожидалось %d: %v", len(regions), len(want), regions)
	}
	for i, w := range want {
		r := regions[i].(map[string]any)
		if r["kind"] != w.kind || r["offset"].(float64) != w.offset || r["size"].(float64) != w.size {
			t.Errorf("регион %d: kind=%v offset=%v size=%v, ожидалось %+v",
				i, r["kind"], r["offset"], r["size"], w)
		}
		if w.name != "" && r["name"] != w.name {
			t.Errorf("регион %d: name=%v, ожидалось %s", i, r["name"], w.name)
		}
	}
	// у дыры должна быть причина-урок
	if note := regions[2].(map[string]any)["note"].(string); note == "" {
		t.Error("padding-регион без note: урок о причине дыры потерян")
	}
}

// Пустые срезы — [], не null: фронту не нужны null-проверки.
func TestJSONEmptySlicesNotNull(t *testing.T) {
	env := envelopeOf(t, []*Model{OfType(reflect.TypeOf(0), "")})
	m := env["models"].([]any)[0].(map[string]any)
	for _, key := range []string{"embeds", "ifaces", "sats", "notes"} {
		v, present := m[key]
		if !present {
			t.Errorf("ключа %q нет в модели", key)
			continue
		}
		if _, isArr := v.([]any); !isArr {
			t.Errorf("%q = %v (%T), ожидался массив []", key, v, v)
		}
	}
	// тип без объекта: адреса нет — пустая строка, не "0x0"
	if m["addr"] != "" {
		t.Errorf("addr паспорта типа = %v, ожидалась пустая строка", m["addr"])
	}
	if m["has_value"] != false {
		t.Error("has_value = true у паспорта типа")
	}
}

// Дамп гиганта усечён при сериализации: конверт обязан выживать под
// потолками транспорта, неся первые jsonBytesCap байт, а не гибнуть целиком.
func TestJSONBytesCapped(t *testing.T) {
	giant := [64 << 10]byte{}
	env := envelopeOf(t, []*Model{Of(&giant, "гигант")})
	m := env["models"].([]any)[0].(map[string]any)
	if hexLen := len(m["bytes"].(string)); hexLen != 2*jsonBytesCap {
		t.Errorf("hex-дамп %d символов, ожидалось %d (потолок)", hexLen, 2*jsonBytesCap)
	}
	// усечение видно потребителю: байт меньше, чем size в паспорте
	if size := m["passport"].(map[string]any)["size"].(float64); int(size) != 64<<10 {
		t.Errorf("паспорт потерял настоящий размер: %v", size)
	}
}

func TestJSONMultipleModels(t *testing.T) {
	a, b := 1, "два"
	env := envelopeOf(t, []*Model{Of(&a, "первый"), Of(&b, "второй")})
	ms := env["models"].([]any)
	if len(ms) != 2 {
		t.Fatalf("моделей %d, ожидалось 2", len(ms))
	}
	if ms[0].(map[string]any)["label"] != "первый" || ms[1].(map[string]any)["label"] != "второй" {
		t.Error("порядок моделей не совпадает с порядком корней")
	}
}
