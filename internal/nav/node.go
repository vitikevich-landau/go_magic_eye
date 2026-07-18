// Package nav — навигационный граф странствия: ленивое дерево узлов поверх
// живых reflect.Value. Ничего не рисует; дерево обходит tui, детали узла
// строит model.
package nav

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/vitikevich-landau/go_magic_eye/internal/model"
	"github.com/vitikevich-landau/go_magic_eye/internal/text"
)

// PageSize — элементов коллекции на одну страницу дерева.
const PageSize = 100

// Node — узел странствия.
type Node struct {
	Label    string        // «banner», «[42]», «Unit», «➤ *main.Knight»
	Sub      string        // короткая аннотация: тип · значение
	Val      reflect.Value // живое (адресуемое, где это возможно) значение
	TypeOnly reflect.Type  // узел «тип без объекта» (галерея AddType)
	Parent   *Node
	Depth    int
	Expanded bool
	Refusal  string // почему узел не раскрыть («nil», «тип стёрт», …)
	Cycle    *Node  // переход ведёт к уже показанному узлу (⟲ цикл / ≡ разделение)
	Shared   bool   // оригинал Cycle — НЕ предок: разделяемая ссылка (ромб/DAG), не цикл
	Copied   string // пометка «это копия, не живая память» (map-значения)

	kids       []*Node
	built      bool
	sess       *Session
	buildRange func(*Node) []*Node // страницы коллекций строят свой диапазон

	detail *model.Model // кэш деталей
}

// HasKids — узел в принципе раскрываем (не считая уже построенных детей).
func (n *Node) HasKids() bool {
	if n.built {
		return len(n.kids) > 0
	}
	if n.buildRange != nil {
		return true
	}
	if n.Refusal != "" || n.Cycle != nil {
		return false
	}
	if n.TypeOnly != nil {
		return n.TypeOnly.Kind() == reflect.Struct
	}
	if !n.Val.IsValid() {
		return false
	}
	switch n.Val.Kind() {
	case reflect.Struct:
		return n.Val.NumField() > 0
	case reflect.Pointer, reflect.Interface:
		return !n.Val.IsNil()
	case reflect.Slice, reflect.Array:
		return n.Val.Len() > 0
	case reflect.Map:
		return !n.Val.IsNil() && n.Val.Len() > 0
	}
	return false
}

// Kids — дети (ленивое строительство при первом обращении).
func (n *Node) Kids() []*Node {
	if !n.built {
		if n.buildRange != nil {
			n.kids = n.buildRange(n)
		} else {
			n.kids = n.sess.buildKids(n)
		}
		n.built = true
	}
	return n.kids
}

// Detail — модель узла для панели деталей (кэшируется).
func (n *Node) Detail() *model.Model {
	if n.detail == nil {
		label := n.Label
		if n.Copied != "" {
			label += " (копия)"
		}
		if n.Cycle != nil {
			where := strings.Join(n.Cycle.Path(), text.Rune(" › ", " > "))
			if n.Shared {
				n.detail = &model.Model{Label: "разделяемая ссылка " + text.Rune("≡", "&"), Notes: []string{
					"этот объект уже показан в дереве другим путём: " + where + ".",
					"оригинал — НЕ предок узла: это второй путь к тому же объекту (ромб/DAG), а не подъём по собственной ветке.",
					"Enter — прыжок к существующему узлу; b — назад.",
				}}
			} else {
				n.detail = &model.Model{Label: "цикл " + text.Rune("⟲", "@"), Notes: []string{
					"указатель ведёт к предку узла — настоящий цикл: " + where + ".",
					"дублей Око не плодит: Enter — прыжок к существующему узлу; b — назад.",
				}}
			}
		} else if n.TypeOnly != nil {
			n.detail = model.OfType(n.TypeOnly, label)
		} else if n.Val.IsValid() {
			n.detail = model.OfValue(n.Val, label)
			if n.Copied != "" {
				n.detail.Notes = append(n.detail.Notes, text.Rune("⚠", "!")+" "+n.Copied)
			}
		} else {
			n.detail = &model.Model{Label: label}
		}
	}
	return n.detail
}

// Explain — почему узел не раскрывается: готовый Refusal (если дети уже
// строились) или причина по типу. Пустая строка = узел раскрываем или
// просто лист без драмы. Нужен TUI: Enter на тупике должен ГОВОРИТЬ, а не
// молчать — «честные отказы» из README обязаны быть видимыми.
func (n *Node) Explain() string {
	if n.Refusal != "" {
		return n.Refusal
	}
	if n.TypeOnly != nil || !n.Val.IsValid() {
		return ""
	}
	switch n.Val.Kind() {
	case reflect.Pointer:
		if n.Val.IsNil() {
			return "nil: идти некуда"
		}
	case reflect.Interface:
		if n.Val.IsNil() {
			return "nil интерфейс: ни типа, ни данных"
		}
	case reflect.Map:
		if n.Val.IsNil() {
			return "nil map"
		}
	case reflect.UnsafePointer:
		return "unsafe.Pointer: тип стёрт — Око по нему не пойдёт"
	case reflect.Func:
		return "функция: код, не данные (имя видно в деталях)"
	case reflect.Chan:
		return "канал: содержимое живёт в hchan, читать его — украсть у горутин"
	}
	return ""
}

// Path — метки узла и всех его предков, от корня к самому узлу. Пища для
// строки-крошек TUI и снимка-документа: ответ на «где я», не зависящий от
// прокрутки.
func (n *Node) Path() []string {
	var segs []string
	for p := n; p != nil; p = p.Parent {
		segs = append(segs, p.Label)
	}
	for i, j := 0, len(segs)-1; i < j; i, j = i+1, j-1 {
		segs[i], segs[j] = segs[j], segs[i]
	}
	return segs
}

// Addr — адрес живого значения узла (0, если неадресуемо).
func (n *Node) Addr() uintptr {
	if n.Val.IsValid() && n.Val.CanAddr() {
		return n.Val.UnsafeAddr()
	}
	return 0
}

func fmtSub(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}
	return v.Type().String() + " · " + shortVal(v)
}

func shortVal(v reflect.Value) string {
	r := []rune(model.FmtVal(v))
	if len(r) > 32 {
		return string(r[:32]) + "…"
	}
	return string(r)
}

func addrLabel(v reflect.Value) string {
	if v.CanAddr() {
		return fmt.Sprintf("@0x%x", v.UnsafeAddr())
	}
	return ""
}
