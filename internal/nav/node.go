// Package nav — навигационный граф странствия: ленивое дерево узлов поверх
// живых reflect.Value. Ничего не рисует; дерево обходит tui, детали узла
// строит model.
package nav

import (
	"fmt"
	"reflect"

	"github.com/vitikevich-landau/go_magic_eye/internal/model"
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
	Cycle    *Node  // переход ведёт к уже показанному узлу (⟲)
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
			n.detail = &model.Model{Label: "цикл ⟲", Notes: []string{
				"этот адрес уже показан в дереве — дублей Око не плодит.",
				"Enter — прыжок к существующему узлу; b — назад.",
			}}
		} else if n.TypeOnly != nil {
			n.detail = model.OfType(n.TypeOnly, label)
		} else if n.Val.IsValid() {
			n.detail = model.OfValue(n.Val, label)
			if n.Copied != "" {
				n.detail.Notes = append(n.detail.Notes, "⚠ "+n.Copied)
			}
		} else {
			n.detail = &model.Model{Label: label}
		}
	}
	return n.detail
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
