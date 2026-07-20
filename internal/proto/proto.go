// Package proto — сеансовый протокол странствия: машинный двойник TUI.
//
// Когда программе выставлен EYE_SESSION=1, Gallery.Run() вместо терминала
// говорит JSON'ом: по stdin приходят команды, в stdout уходят ответы —
// по строке на сообщение. Поверх этого playground строит странствие в
// браузере, но протокол самостоятелен: им может пользоваться любой клиент
// (редакторный плагин, отладочный скрипт).
//
// Рукопожатие: первой строкой сеанс печатает hello
//
//	{"eye_session_version": 1, "roots": [узлы…]}
//
// Дальше — цикл запрос/ответ (id связывает пары):
//
//	→ {"id": 1, "cmd": "kids",   "node": 3}   дети узла (ленивое построение)
//	← {"id": 1, "ok": true, "nodes": [узлы…]}
//	→ {"id": 2, "cmd": "detail", "node": 3}   Гримуар узла
//	← {"id": 2, "ok": true, "eye": {конверт c одной моделью}}
//	→ {"id": 3, "cmd": "quit"}
//	← {"id": 3, "ok": true}                   …и процесс завершается
//
// Отказ — {"id": N, "ok": false, "error": "почему"}. Узлы получают числовые
// id по мере того, как впервые уходят клиенту; id стабильны до конца сеанса.
//
// Контракт тот же, что у JSON-вида (playground/SPEC.md): поля добавлять
// можно, менять смысл — только со сменой eye_session_version.
package proto

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/vitikevich-landau/go_magic_eye/internal/model"
	"github.com/vitikevich-landau/go_magic_eye/internal/nav"
)

// Version — версия протокола сеанса.
const Version = 1

// NodeDTO — узел дерева, как его видит клиент.
type NodeDTO struct {
	ID         int    `json:"id"`
	Label      string `json:"label"`
	Sub        string `json:"sub"`
	Expandable bool   `json:"expandable"`
	// Refusal — почему узел не раскрыть (nil, тип стёрт, функция…);
	// пустая строка — раскрываем или обычный лист
	Refusal string `json:"refusal"`
	// Cycle — id узла-оригинала: переход ведёт к уже показанному месту.
	// 0 — обычный узел. Shared уточняет сорт: true — разделяемая ссылка ≡
	// (ромб/DAG), false — настоящий цикл ⟲ (оригинал — предок)
	Cycle  int    `json:"cycle"`
	Shared bool   `json:"shared"`
	Copied string `json:"copied"`
}

type request struct {
	ID   int    `json:"id"`
	Cmd  string `json:"cmd"`
	Node int    `json:"node"`
}

type response struct {
	ID    int             `json:"id"`
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Nodes []NodeDTO       `json:"nodes,omitempty"`
	Eye   json.RawMessage `json:"eye,omitempty"`
}

type hello struct {
	Version int       `json:"eye_session_version"`
	Roots   []NodeDTO `json:"roots"`
}

// server — состояние одного сеанса: реестр узлов с двусторонней картой.
type server struct {
	sess   *nav.Session
	out    io.Writer
	byID   map[int]*nav.Node
	byNode map[*nav.Node]int
	nextID int
}

// Run — сеанс на паре потоков: блокирует до quit или конца stdin.
// Ошибки записи в out означают смерть клиента — сеанс молча завершается.
func Run(sess *nav.Session, in io.Reader, out io.Writer) {
	s := &server{
		sess:   sess,
		out:    out,
		byID:   map[int]*nav.Node{},
		byNode: map[*nav.Node]int{},
	}
	roots := make([]NodeDTO, 0, len(sess.Roots))
	for _, r := range sess.Roots {
		roots = append(roots, s.dto(r))
	}
	// hello обязан начаться с чистой строки: программа могла напечатать
	// что-то без завершающего \n (fmt.Print) прямо перед странствием, и
	// без этого разрыва рукопожатие приклеилось бы к чужому хвосту
	if _, err := s.out.Write([]byte("\n")); err != nil {
		return
	}
	if !s.write(hello{Version: Version, Roots: roots}) {
		return
	}

	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			if !s.write(response{ID: req.ID, Error: "нечитаемая команда: " + err.Error()}) {
				return
			}
			continue
		}
		resp, quit := s.handle(req)
		if !s.write(resp) {
			return
		}
		if quit {
			return
		}
	}
}

func (s *server) handle(req request) (response, bool) {
	switch req.Cmd {
	case "quit":
		return response{ID: req.ID, OK: true}, true

	case "kids":
		n, err := s.node(req.Node)
		if err != "" {
			return response{ID: req.ID, Error: err}, false
		}
		if !n.HasKids() {
			// честный отказ — та же речь, что у Enter в TUI
			why := n.Explain()
			if why == "" {
				why = "лист: раскрывать нечего"
			}
			return response{ID: req.ID, Error: why}, false
		}
		kids := n.Kids()
		nodes := make([]NodeDTO, 0, len(kids))
		for _, k := range kids {
			nodes = append(nodes, s.dto(k))
		}
		return response{ID: req.ID, OK: true, Nodes: nodes}, false

	case "detail":
		n, err := s.node(req.Node)
		if err != "" {
			return response{ID: req.ID, Error: err}, false
		}
		env, jerr := model.ToJSON([]*model.Model{n.Detail()})
		if jerr != nil {
			return response{ID: req.ID, Error: "модель не сериализовалась: " + jerr.Error()}, false
		}
		return response{ID: req.ID, OK: true, Eye: env}, false
	}
	return response{ID: req.ID, Error: fmt.Sprintf("неизвестная команда %q", req.Cmd)}, false
}

func (s *server) node(id int) (*nav.Node, string) {
	n, ok := s.byID[id]
	if !ok {
		return nil, fmt.Sprintf("узла %d нет в сеансе", id)
	}
	return n, ""
}

// dto — узел наружу; при первой встрече узел получает id.
func (s *server) dto(n *nav.Node) NodeDTO {
	id, seen := s.byNode[n]
	if !seen {
		s.nextID++
		id = s.nextID
		s.byNode[n] = id
		s.byID[id] = n
	}
	d := NodeDTO{
		ID:         id,
		Label:      n.Label,
		Sub:        n.Sub,
		Expandable: n.HasKids(),
		Refusal:    n.Explain(),
		Shared:     n.Shared,
		Copied:     n.Copied,
	}
	if n.Cycle != nil {
		cycleDTO := s.dto(n.Cycle) // оригинал уже показан — id у него есть или появится
		d.Cycle = cycleDTO.ID
		d.Expandable = false
	}
	return d
}

func (s *server) write(v any) bool {
	b, err := json.Marshal(v)
	if err != nil {
		return true // невозможно по построению; строку не шлём, сеанс жив
	}
	b = append(b, '\n')
	_, werr := s.out.Write(b)
	return werr == nil
}
