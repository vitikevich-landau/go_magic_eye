package model

import (
	"fmt"
	"reflect"
	"unsafe"
)

// Анатомия interface-значения.
//
// Значение интерфейса в Go — ДВА машинных слова:
//
//	any / eface:            iface (есть методы):
//	  [0] *_type              [0] *itab   ← «vtable» Go
//	  [1] data                [1] data
//
// itab (internal/abi.ITab, стабилен в gc-рантайме много лет):
//
//	+0  inter *interfacetype
//	+8  _type *_type
//	+16 hash  uint32          (копия хеша типа — ускоряет type switch)
//	+20 _     [4]byte
//	+24 fun   [N]uintptr      ← адреса методов, отсортированы по имени
//
// Чтение fun — прогулка по внутренностям рантайма: легально по языку
// (unsafe), но раскладка не обещана спецификацией. Око читает её только на
// 64-битных платформах и честно подписывает происхождение знания.
const itabFunOffset = 24

var is64 = wordSize == 8

// ifaceInfo собирает анатомию интерфейсного значения v (тип t — статический
// тип интерфейса). v должен быть addressable — слова читаются прямо из памяти.
func (b *builder) ifaceInfo(t reflect.Type, v reflect.Value, where string) Iface {
	info := Iface{
		Where:    where,
		Empty:    t.NumMethod() == 0,
		TypeName: typeName(t),
		DynType:  "⌀ nil",
	}
	// два сырых слова — прямо из памяти объекта
	var tabP unsafe.Pointer
	if v.CanAddr() {
		p := v.Addr().UnsafePointer()
		tabP = *(*unsafe.Pointer)(p)
		info.TabAddr = uintptr(tabP)
		info.DataAddr = uintptr(*(*unsafe.Pointer)(unsafe.Add(p, wordSize)))
	}
	if v.IsNil() {
		info.Note = "оба слова нулевые: нет ни типа, ни данных — вот что такое «nil интерфейс»"
		return info
	}
	dyn := v.Elem()
	info.DynType = typeName(dyn.Type())
	switch dyn.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		if dyn.IsNil() {
			info.TypedNil = true
			info.Note = "слово типа заполнено, а data — nil: интерфейс != nil, хотя внутри пустой указатель. " +
				"Классическая ловушка «err != nil, а ошибки нет»"
		}
	}

	// методы: имена — из статического типа интерфейса (отсортированы, как fun)
	for i := 0; i < t.NumMethod(); i++ {
		info.Methods = append(info.Methods, Method{Name: t.Method(i).Name})
	}
	if info.Empty {
		info.Note = firstNonEmpty(info.Note,
			"eface: первое слово — сам *_type (таблицы методов нет: у any методов нет)")
		return info
	}

	// сырое чтение itab: hash + fun[i] → runtime.FuncForPC
	if is64 && tabP != nil {
		info.Hash = *(*uint32)(unsafe.Add(tabP, 2*wordSize))
		for i := range info.Methods {
			pc := *(*uintptr)(unsafe.Add(tabP, itabFunOffset+uintptr(i)*wordSize))
			info.Methods[i].PC = pc
			info.Methods[i].Func = funcName(pc)
		}
		info.Note = firstNonEmpty(info.Note,
			"слоты fun прочитаны из живого itab (раскладка gc-рантайма go1.21+; спецификация её не обещает)")
	} else if !is64 {
		info.Note = firstNonEmpty(info.Note,
			"32-битная платформа: сырые слоты itab Око не читает — раскладка другая; имена методов — из reflect")
	}
	return info
}

// IfaceOfRoot — анатомия интерфейса-корня (Inspect(&s), где s — interface).
func IfaceOfRoot(t reflect.Type, v reflect.Value) Iface {
	b := &builder{m: &Model{HasValue: true}}
	return b.ifaceInfo(t, v, "объект целиком")
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// DynDataValue — живое динамическое значение интерфейса для странствия.
//
// Если данные лежат за указателем (data — адрес), возвращаем значение ПО МЕСТУ
// (NewAt — живая память коробки). Если значение pointer-shaped (сам ptr/map/
// chan/func — data и есть значение), синтезируем его копию: она укажет туда же.
func DynDataValue(v reflect.Value) (reflect.Value, string, bool) {
	v = readable(v)
	if v.Kind() != reflect.Interface || v.IsNil() {
		return reflect.Value{}, "", false
	}
	dyn := v.Elem()
	dt := dyn.Type()
	if !v.CanAddr() {
		box := reflect.New(dyn.Type())
		box.Elem().Set(reflect.ValueOf(dynIface(dyn)))
		return box.Elem(), "копия (интерфейс не был адресуем)", true
	}
	p := unsafe.Pointer(v.UnsafeAddr())
	data := *(*unsafe.Pointer)(unsafe.Add(p, wordSize))
	switch dt.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		// pointer-shaped: слово data — само значение
		box := reflect.New(dt)
		*(*unsafe.Pointer)(unsafe.Pointer(box.Pointer())) = data
		return box.Elem(), "pointer-shaped: слово data — само значение", true
	default:
		if data == nil {
			return reflect.Value{}, "", false
		}
		return reflect.NewAt(dt, data).Elem(), fmt.Sprintf("живые данные коробки @0x%x", uintptr(data)), true
	}
}

func dynIface(dyn reflect.Value) any {
	if dyn.CanInterface() {
		return dyn.Interface()
	}
	box := reflect.New(dyn.Type())
	box.Elem().Set(dyn) // не случится: dyn от readable
	return box.Elem().Interface()
}
