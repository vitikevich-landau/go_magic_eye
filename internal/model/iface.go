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

// isDirectIface — хранится ли значение типа t в слове data ПРЯМО (а не за
// указателем). Это точная копия правила runtime (isdirectiface): прямыми
// бывают не только ptr/map/chan/func/unsafe.Pointer, но и — сюрприз —
// структура с ЕДИНСТВЕННЫМ прямым полем (struct{ p *T }) и массив [1]T из
// прямого T: их представление — одно слово-указатель, и рантайм кладёт его
// в data как есть. Прочитать такое NewAt'ом по «адресу» data — значит
// принять сам указатель за адрес структуры и уехать в чужую память.
func isDirectIface(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return true
	case reflect.Struct:
		return t.NumField() == 1 && isDirectIface(t.Field(0).Type)
	case reflect.Array:
		return t.Len() == 1 && isDirectIface(t.Elem())
	}
	return false
}

// DynDataValue — живое динамическое значение интерфейса для странствия.
//
// Если данные лежат за указателем (data — адрес коробки), возвращаем значение
// ПО МЕСТУ (NewAt — живая память). Если тип direct-iface (см. isDirectIface —
// слово data и ЕСТЬ значение), синтезируем коробку и кладём слово в неё:
// содержимое укажет туда же, куда и оригинал.
func DynDataValue(v reflect.Value) (reflect.Value, string, bool) {
	v = readable(v)
	if v.Kind() != reflect.Interface || v.IsNil() {
		return reflect.Value{}, "", false
	}
	dyn := v.Elem()
	dt := dyn.Type()
	if !v.CanAddr() {
		// интерфейс не адресуем (теоретический путь): без адреса слово data
		// не прочитать; берём значение через Interface, если позволено
		if !dyn.CanInterface() {
			return reflect.Value{}, "", false
		}
		box := reflect.New(dt)
		box.Elem().Set(reflect.ValueOf(dyn.Interface()))
		return box.Elem(), "копия (интерфейс не был адресуем)", true
	}
	p := v.Addr().UnsafePointer()
	data := *(*unsafe.Pointer)(unsafe.Add(p, wordSize))
	if isDirectIface(dt) {
		box := reflect.New(dt)
		*(*unsafe.Pointer)(box.UnsafePointer()) = data
		return box.Elem(), "direct-iface: слово data — само значение", true
	}
	if data == nil {
		return reflect.Value{}, "", false
	}
	return reflect.NewAt(dt, data).Elem(), fmt.Sprintf("живые данные коробки @0x%x", uintptr(data)), true
}
