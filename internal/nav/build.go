package nav

import (
	"fmt"
	"github.com/vitikevich-landau/go_magic_eye/internal/text"
	"reflect"

	"github.com/vitikevich-landau/go_magic_eye/internal/model"
)

// buildKids — ленивое строительство детей узла. Всё типизировано: Око знает,
// что за узлом, и честно отказывается идти туда, где типа нет.
//
// Ленивость — не оптимизация ради галочки, а условие выживания: граф
// достижимых значений может быть огромным (срез на миллион элементов) или
// вовсе бесконечным (циклы указателей). Поэтому дети строятся ровно один
// раз и ровно тогда, когда пользователь раскрыл узел; до того узел — только
// ярлык с превью.
func (s *Session) buildKids(n *Node) []*Node {
	if n.TypeOnly != nil {
		return s.typeKids(n)
	}
	v := n.Val
	if !v.IsValid() {
		return nil
	}
	v = model.Readable(v)
	switch v.Kind() {
	case reflect.Struct:
		return s.structKids(n, v)
	case reflect.Pointer:
		return s.ptrKids(n, v)
	case reflect.Interface:
		return s.ifaceKids(n, v)
	case reflect.Slice, reflect.Array:
		return s.seqKids(n, v, 0)
	case reflect.Map:
		return s.mapKids(n, v)
	case reflect.String:
		return nil // байты строки видны в деталях; отдельных детей не плодим
	}
	return nil
}

func (s *Session) child(parent *Node, label, sub string, v reflect.Value) *Node {
	return &Node{
		Label: label, Sub: sub, Val: v,
		Parent: parent, Depth: parent.Depth + 1, sess: s,
	}
}

// structKids — встроенные типы (композиция) первыми, затем собственные поля.
func (s *Session) structKids(n *Node, v reflect.Value) []*Node {
	t := v.Type()
	var out []*Node
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		label := f.Name
		if f.Anonymous {
			label = text.Rune("▣ ", "= ") + f.Type.String()
		}
		c := s.child(n, label, fmtSub(fv), fv)
		if f.Anonymous {
			c.Sub = fmt.Sprintf("встроен @ +%d", f.Offset)
		}
		out = append(out, c)
	}
	return out
}

// ptrKids — единственный ребёнок: цель указателя (живая память).
//
// v.Elem() у ненулевого указателя ВСЕГДА адресуем (память за указателем —
// вот она), поэтому цель показывается по месту, без копий, и её собственные
// указатели ведут дальше — так странствие и «идёт по кладбищу ссылок».
func (s *Session) ptrKids(n *Node, v reflect.Value) []*Node {
	if v.IsNil() {
		n.Refusal = "nil: идти некуда"
		return nil
	}
	target := v.Elem()
	key := seenKey{addr: v.Pointer(), t: target.Type()}
	if prev, ok := s.seen[key]; ok && prev != n {
		c := s.child(n, "уже показан", target.Type().String()+" · Enter — прыжок", reflect.Value{})
		c.Cycle = prev
		return []*Node{c}
	}
	c := s.child(n, text.Rune("➤", "->")+" цель "+addrLabel(target), fmtSub(target), target)
	s.remember(c)
	return []*Node{c}
}

// ifaceKids — динамическое значение интерфейса.
func (s *Session) ifaceKids(n *Node, v reflect.Value) []*Node {
	if v.IsNil() {
		n.Refusal = "nil интерфейс: ни типа, ни данных"
		return nil
	}
	dyn, how, ok := model.DynDataValue(v)
	if !ok {
		n.Refusal = "данных нет (typed nil?)"
		return nil
	}
	// интерфейс может указывать на уже показанный объект — цикл ловится
	// так же, как у указателей (иначе self-ссылки через any плодили бы дубли)
	if dyn.CanAddr() {
		key := seenKey{addr: dyn.UnsafeAddr(), t: dyn.Type()}
		if prev, seen := s.seen[key]; seen {
			c := s.child(n, "уже показан", dyn.Type().String()+" · Enter — прыжок", reflect.Value{})
			c.Cycle = prev
			return []*Node{c}
		}
	}
	c := s.child(n, text.Rune("◈", "*")+" динамика: "+dyn.Type().String(), how, dyn)
	if dyn.CanAddr() {
		s.remember(c)
	}
	return []*Node{c}
}

// seqKids — элементы среза/массива; длинные — страницами по PageSize.
func (s *Session) seqKids(n *Node, v reflect.Value, base int) []*Node {
	ln := v.Len()
	if ln == 0 {
		return nil
	}
	if ln > PageSize {
		var out []*Node
		for p := 0; p < ln; p += PageSize {
			hi := min(p+PageSize, ln)
			pg := s.child(n, fmt.Sprintf("%s [%d..%d]", text.Rune("⁘", ".."), p, hi-1),
				fmt.Sprintf("страница из %d", ln), v)
			lo := p
			pg.built = false
			pg.sess = s
			// страница строит свой диапазон сама
			pgLo, pgHi := lo, hi
			pg.buildRange = func(pn *Node) []*Node { return s.rangeKids(pn, v, pgLo, pgHi) }
			out = append(out, pg)
		}
		return out
	}
	return s.rangeKids(n, v, 0, ln)
}

func (s *Session) rangeKids(n *Node, v reflect.Value, lo, hi int) []*Node {
	// живой срез мог укоротиться с момента, когда страницы были нарезаны
	// (Око смотрит на живую память — пользовательский код имеет право
	// мутировать её между кадрами); за пределы len не выходим
	if l := v.Len(); hi > l {
		hi = l
	}
	var out []*Node
	for i := lo; i < hi; i++ {
		ev := v.Index(i)
		out = append(out, s.child(n, fmt.Sprintf("[%d]", i), fmtSub(ev), ev))
	}
	if len(out) == 0 {
		n.Refusal = "срез укоротился: этой страницы больше нет"
	}
	return out
}

// mapKids — пары map; значения НЕ адресуемы — честные копии с пометкой.
//
// Почему язык запрещает &m[k]: map при росте перевешивает бакеты в новую
// память, и взятый адрес молча протух бы. Поэтому единственный честный
// способ показать значение — скопировать его; пометка Copied прямо говорит
// пользователю, что он смотрит на снимок, а не на живую память.
func (s *Session) mapKids(n *Node, v reflect.Value) []*Node {
	if v.IsNil() {
		n.Refusal = "nil map"
		return nil
	}
	var out []*Node
	it := v.MapRange()
	i := 0
	for it.Next() {
		if i >= PageSize {
			more := s.child(n, fmt.Sprintf("%s ещё %d пар", text.Rune("⋯", "..."), v.Len()-i), "порядок случаен", reflect.Value{})
			more.Refusal = "Око показывает первые " + fmt.Sprint(PageSize) + " пар"
			out = append(out, more)
			break
		}
		box := reflect.New(it.Value().Type()).Elem()
		box.Set(model.Readable(it.Value()))
		c := s.child(n, text.Rune("⚷ ", "k:")+shortVal(it.Key()), fmtSub(box), box)
		c.Copied = "значения map не адресуемы: это копия (map может перевесить бакеты в любой момент)"
		out = append(out, c)
		i++
	}
	return out
}

// typeKids — дети узла «тип без объекта»: поля структуры, тоже без объектов.
func (s *Session) typeKids(n *Node) []*Node {
	t := n.TypeOnly
	if t.Kind() != reflect.Struct {
		return nil
	}
	var out []*Node
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		c := s.child(n, f.Name, fmt.Sprintf("%s @ +%d", f.Type.String(), f.Offset), reflect.Value{})
		c.TypeOnly = f.Type
		out = append(out, c)
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
