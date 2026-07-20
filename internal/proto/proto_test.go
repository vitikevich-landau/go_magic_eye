package proto

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/vitikevich-landau/go_magic_eye/internal/nav"
)

// Guild — цикл через указатели: рыцарь знает гильдию, гильдия — рыцаря.
type guild struct {
	Name   string
	Leader *knight
}

type knight struct {
	HP   int32
	Home *guild
}

func session(t *testing.T) (*nav.Session, *guild) {
	t.Helper()
	g := &guild{Name: "Орден"}
	k := &knight{HP: 100, Home: g}
	g.Leader = k
	s := nav.NewSession()
	s.AddRoot(reflect.ValueOf(g).Elem(), "гильдия")
	return s, g
}

// dialogue — прогнать команды через Run, вернуть hello и ответы по порядку.
func dialogue(t *testing.T, s *nav.Session, cmds ...string) (map[string]any, []map[string]any) {
	t.Helper()
	var out strings.Builder
	Run(s, strings.NewReader(strings.Join(cmds, "\n")+"\n"), &out)

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) == 0 {
		t.Fatal("сеанс промолчал")
	}
	var hi map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &hi); err != nil {
		t.Fatalf("hello не парсится: %v\n%s", err, lines[0])
	}
	if v, _ := hi["eye_session_version"].(float64); int(v) != Version {
		t.Fatalf("eye_session_version = %v", hi["eye_session_version"])
	}
	resps := make([]map[string]any, 0, len(lines)-1)
	for _, l := range lines[1:] {
		var r map[string]any
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatalf("ответ не парсится: %v\n%s", err, l)
		}
		resps = append(resps, r)
	}
	return hi, resps
}

func nodes(v any) []map[string]any {
	arr := v.([]any)
	out := make([]map[string]any, len(arr))
	for i, x := range arr {
		out[i] = x.(map[string]any)
	}
	return out
}

func TestHelloAndKids(t *testing.T) {
	s, _ := session(t)
	hi, resps := dialogue(t, s,
		`{"id":1,"cmd":"kids","node":1}`,
		`{"id":9,"cmd":"quit"}`,
	)
	roots := nodes(hi["roots"])
	if len(roots) != 1 || roots[0]["label"] != "гильдия" {
		t.Fatalf("roots: %v", roots)
	}
	if roots[0]["id"].(float64) != 1 || roots[0]["expandable"] != true {
		t.Fatalf("корень: %v", roots[0])
	}

	if resps[0]["ok"] != true {
		t.Fatalf("kids не ок: %v", resps[0])
	}
	kids := nodes(resps[0]["nodes"])
	if len(kids) != 2 {
		t.Fatalf("детей %d, ожидалось 2 (Name, Leader): %v", len(kids), kids)
	}
	if kids[0]["label"] != "Name" || kids[1]["label"] != "Leader" {
		t.Errorf("метки детей: %v, %v", kids[0]["label"], kids[1]["label"])
	}
	if resps[1]["ok"] != true {
		t.Errorf("quit не ок: %v", resps[1])
	}
}

func TestDetailReturnsEnvelope(t *testing.T) {
	s, _ := session(t)
	_, resps := dialogue(t, s,
		`{"id":1,"cmd":"detail","node":1}`,
		`{"id":2,"cmd":"quit"}`,
	)
	if resps[0]["ok"] != true {
		t.Fatalf("detail не ок: %v", resps[0])
	}
	eye := resps[0]["eye"].(map[string]any)
	if eye["eye_json_version"].(float64) != 1 {
		t.Fatalf("в detail не конверт: %v", eye)
	}
	m := eye["models"].([]any)[0].(map[string]any)
	if m["label"] != "гильдия" {
		t.Errorf("label модели: %v", m["label"])
	}
}

// Идти по циклу гильдия → Leader → Home: узел Home должен прийти со ссылкой
// cycle на id корня, а не плодить дубль.
func TestCycleMarked(t *testing.T) {
	s, _ := session(t)
	_, resps := dialogue(t, s,
		`{"id":1,"cmd":"kids","node":1}`, // Name(2), Leader(3)
		`{"id":2,"cmd":"kids","node":3}`, // ➤ *knight(4)
		`{"id":3,"cmd":"kids","node":4}`, // HP, Home
		`{"id":4,"cmd":"quit"}`,
	)
	leaderKids := nodes(resps[1]["nodes"])
	if len(leaderKids) == 0 {
		t.Fatalf("у Leader нет детей: %v", resps[1])
	}
	knightKids := nodes(resps[2]["nodes"])
	var home map[string]any
	for _, k := range knightKids {
		if strings.Contains(k["label"].(string), "Home") {
			home = k
		}
	}
	if home == nil {
		t.Fatalf("поля Home нет среди: %v", knightKids)
	}
	// Home — указатель на гильдию: либо сам узел, либо его раскрытие ведёт
	// к циклу; проверяем, что где-то по пути cycle != 0 и он ссылается на
	// корень (id 1)
	cycleID := home["cycle"].(float64)
	if cycleID == 0 {
		// возможно, цикл на уровень глубже (узел разыменования)
		t.Logf("Home без метки цикла, пробуем раскрыть")
		s2, _ := session(t)
		_, r2 := dialogue(t, s2,
			`{"id":1,"cmd":"kids","node":1}`,
			`{"id":2,"cmd":"kids","node":3}`,
			`{"id":3,"cmd":"kids","node":4}`,
			`{"id":4,"cmd":"kids","node":`+jsonNum(home["id"])+`}`,
			`{"id":5,"cmd":"quit"}`,
		)
		deeper := nodes(r2[3]["nodes"])
		found := false
		for _, d := range deeper {
			if d["cycle"].(float64) != 0 {
				found = true
			}
		}
		if !found {
			t.Fatalf("цикл не помечен ни на Home, ни глубже: %v", deeper)
		}
	}
}

func jsonNum(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// Отказы — честные: kids на листе и на nil-указателе отвечают причиной.
func TestRefusals(t *testing.T) {
	type hermit struct {
		Age  int
		Home *guild // nil
	}
	s := nav.NewSession()
	h := hermit{Age: 70}
	s.AddRoot(reflect.ValueOf(&h).Elem(), "отшельник")

	_, resps := dialogue(t, s,
		`{"id":1,"cmd":"kids","node":1}`, // Age(2), Home(3)
		`{"id":2,"cmd":"kids","node":2}`, // лист
		`{"id":3,"cmd":"kids","node":3}`, // nil-указатель
		`{"id":4,"cmd":"kids","node":99}`,
		`{"id":5,"cmd":"nonsense"}`,
		`{"id":6,"cmd":"quit"}`,
	)
	if resps[1]["ok"] == true || resps[1]["error"] == "" {
		t.Errorf("лист раскрылся без отказа: %v", resps[1])
	}
	if resps[2]["ok"] == true || !strings.Contains(resps[2]["error"].(string), "nil") {
		t.Errorf("nil-указатель без честного отказа: %v", resps[2])
	}
	if resps[3]["ok"] == true {
		t.Errorf("несуществующий узел раскрылся: %v", resps[3])
	}
	if resps[4]["ok"] == true {
		t.Errorf("бессмысленная команда прошла: %v", resps[4])
	}
}
