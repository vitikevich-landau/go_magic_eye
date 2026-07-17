package nav

import (
	"reflect"
	"strings"
)

type seenKey struct {
	addr uintptr
	t    reflect.Type
}

// Session — дерево странствия: корни галереи, плоский срез видимых узлов,
// курсор, история прыжков и учёт уже показанных адресов (циклы ⟲).
type Session struct {
	Roots   []*Node
	Cursor  int
	visible []*Node
	history []*Node
	seen    map[seenKey]*Node
}

func NewSession() *Session {
	return &Session{seen: map[seenKey]*Node{}}
}

// AddRoot — живой корень (значение или указатель — как в model.Of).
func (s *Session) AddRoot(v reflect.Value, label string) *Node {
	n := &Node{Label: label, Sub: fmtSub(v), Val: v, sess: s}
	if label == "" {
		n.Label = v.Type().String()
		n.Sub = shortVal(v)
	}
	s.Roots = append(s.Roots, n)
	s.remember(n)
	s.Refresh()
	return n
}

// AddTypeRoot — корень «тип без объекта».
func (s *Session) AddTypeRoot(t reflect.Type, label string) *Node {
	if label == "" {
		label = t.String()
	}
	n := &Node{Label: "⌘ " + label, Sub: "только статика типа", TypeOnly: t, sess: s}
	s.Roots = append(s.Roots, n)
	s.Refresh()
	return n
}

func (s *Session) remember(n *Node) {
	if a := n.Addr(); a != 0 {
		key := seenKey{addr: a, t: n.Val.Type()}
		if _, dup := s.seen[key]; !dup {
			s.seen[key] = n
		}
	}
}

// Refresh пересчитывает плоский список видимых узлов (после expand/collapse).
func (s *Session) Refresh() {
	s.visible = s.visible[:0]
	var walk func(n *Node)
	walk = func(n *Node) {
		s.visible = append(s.visible, n)
		if n.Expanded {
			for _, k := range n.Kids() {
				walk(k)
			}
		}
	}
	for _, r := range s.Roots {
		walk(r)
	}
	if s.Cursor >= len(s.visible) {
		s.Cursor = len(s.visible) - 1
	}
	if s.Cursor < 0 {
		s.Cursor = 0
	}
}

// Visible — плоский список видимых узлов.
func (s *Session) Visible() []*Node { return s.visible }

// Current — узел под курсором.
func (s *Session) Current() *Node {
	if len(s.visible) == 0 {
		return nil
	}
	return s.visible[s.Cursor]
}

func (s *Session) Move(d int) {
	s.Cursor += d
	if s.Cursor < 0 {
		s.Cursor = 0
	}
	if s.Cursor >= len(s.visible) {
		s.Cursor = len(s.visible) - 1
	}
}

// Enter — раскрыть узел; на узле-цикле ⟲ — прыгнуть к оригиналу.
func (s *Session) Enter() {
	n := s.Current()
	if n == nil {
		return
	}
	if n.Cycle != nil {
		s.JumpTo(n.Cycle)
		return
	}
	if n.HasKids() && !n.Expanded {
		n.Expanded = true
		s.Refresh()
	}
}

// Collapse — свернуть узел или подняться к родителю.
func (s *Session) Collapse() {
	n := s.Current()
	if n == nil {
		return
	}
	if n.Expanded {
		n.Expanded = false
		s.Refresh()
		return
	}
	if n.Parent != nil {
		s.JumpNoHistory(n.Parent)
	}
}

// ExpandAll — рекурсивно раскрыть ветку под курсором (клавиша e).
// Глубина ограничена: странствие ленивое, но e просит содержимого.
func (s *Session) ExpandAll() {
	n := s.Current()
	if n == nil {
		return
	}
	var rec func(n *Node, depth int)
	rec = func(n *Node, depth int) {
		if depth > 4 || !n.HasKids() {
			return
		}
		n.Expanded = true
		for _, k := range n.Kids() {
			rec(k, depth+1)
		}
	}
	rec(n, 0)
	s.Refresh()
}

// CollapseAll — свернуть всё (клавиша c).
func (s *Session) CollapseAll() {
	var rec func(n *Node)
	rec = func(n *Node) {
		n.Expanded = false
		for _, k := range n.kids {
			rec(k)
		}
	}
	for _, r := range s.Roots {
		rec(r)
	}
	s.Cursor = 0
	s.Refresh()
}

// JumpTo — прыжок к узлу с записью в историю (переходы по ⟲ и поиску).
func (s *Session) JumpTo(target *Node) {
	if cur := s.Current(); cur != nil {
		s.history = append(s.history, cur)
	}
	s.reveal(target)
}

// Back — назад по истории (клавиша b/⌫).
func (s *Session) Back() {
	if len(s.history) == 0 {
		return
	}
	target := s.history[len(s.history)-1]
	s.history = s.history[:len(s.history)-1]
	s.reveal(target)
}

// JumpNoHistory — тихий переход (например, к родителю).
func (s *Session) JumpNoHistory(target *Node) { s.reveal(target) }

// reveal раскрывает предков цели и ставит на неё курсор.
func (s *Session) reveal(target *Node) {
	for p := target.Parent; p != nil; p = p.Parent {
		p.Expanded = true
	}
	s.Refresh()
	for i, n := range s.visible {
		if n == target {
			s.Cursor = i
			return
		}
	}
}

// JumpRoot — прыжок к N-му корню галереи (клавиши 1..9).
func (s *Session) JumpRoot(i int) {
	if i >= 0 && i < len(s.Roots) {
		s.JumpTo(s.Roots[i])
	}
}

// Search — поиск по видимым (раскрытым) узлам, от следующего за курсором.
func (s *Session) Search(query string, backwards bool) bool {
	if query == "" || len(s.visible) == 0 {
		return false
	}
	q := strings.ToLower(query)
	n := len(s.visible)
	step := 1
	if backwards {
		step = n - 1
	}
	for off := 1; off <= n; off++ {
		i := (s.Cursor + off*step) % n
		node := s.visible[i]
		if strings.Contains(strings.ToLower(node.Label), q) ||
			strings.Contains(strings.ToLower(node.Sub), q) {
			s.Cursor = i
			return true
		}
	}
	return false
}
