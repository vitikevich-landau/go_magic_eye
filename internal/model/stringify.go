package model

import (
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"unsafe"
)

// readable снимает с адресуемого значения запрет reflect на чтение
// неэкспортированных полей: NewAt по живому адресу даёт «чистый» Value.
// Это легальный приём (unsafe, но задокументированный) — и главный учебный
// пункт: в Go рефлексия видит ВСЁ, макросы-реестры не нужны.
func readable(v reflect.Value) reflect.Value {
	if v.CanInterface() {
		return v
	}
	if v.CanAddr() {
		return reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
	}
	return v
}

// typeName — человеческое имя типа. reflect.Type.String() уже читаем
// (деманглер не нужен — ещё одно отличие от C++).
func typeName(t reflect.Type) string {
	if t == nil {
		return "nil"
	}
	return t.String()
}

// kindName — reflect.Kind по-русски.
func kindName(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Struct:
		return "структура"
	case reflect.Pointer:
		return "указатель"
	case reflect.Interface:
		if t.NumMethod() == 0 {
			return "пустой интерфейс (eface)"
		}
		return "интерфейс с методами (iface)"
	case reflect.String:
		return "строка"
	case reflect.Slice:
		return "срез"
	case reflect.Array:
		return "массив"
	case reflect.Map:
		return "map (хеш-таблица)"
	case reflect.Chan:
		return "канал"
	case reflect.Func:
		return "функция"
	case reflect.Bool:
		return "булево"
	case reflect.Uintptr:
		return "uintptr (голый адрес)"
	case reflect.UnsafePointer:
		return "unsafe.Pointer"
	case reflect.Float32, reflect.Float64:
		return "вещественное"
	case reflect.Complex64, reflect.Complex128:
		return "комплексное"
	}
	if k := t.Kind(); k >= reflect.Int && k <= reflect.Uint64 {
		return "целое"
	}
	return t.Kind().String()
}

// quote — строка для превью: экранированная, обрезанная до max рун.
func quote(s string, max int) string {
	r := []rune(s)
	if len(r) > max {
		return strconv.Quote(string(r[:max])) + "…"
	}
	return strconv.Quote(s)
}

// funcName — имя функции по адресу кода (runtime.FuncForPC).
func funcName(pc uintptr) string {
	if pc == 0 {
		return "?"
	}
	if f := runtime.FuncForPC(pc); f != nil {
		return f.Name()
	}
	return "?"
}

// FmtVal — краткое строковое значение (для дерева странствия).
func FmtVal(v reflect.Value) string { return fmtVal(v, 0) }

// Readable — публичная обёртка readable для пакета nav.
func Readable(v reflect.Value) reflect.Value { return readable(v) }

// fmtVal — краткое строковое значение для превью/элементов.
// Работает и на неэкспортированных значениях: только kind-геттеры reflect.
func fmtVal(v reflect.Value, depth int) string {
	if !v.IsValid() {
		return "nil"
	}
	v = readable(v)
	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			return "true"
		}
		return "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, 64)
	case reflect.Complex64, reflect.Complex128:
		return fmt.Sprint(v.Complex())
	case reflect.String:
		return quote(v.String(), 24)
	case reflect.Pointer, reflect.UnsafePointer:
		if v.IsNil() {
			return "nil"
		}
		return fmt.Sprintf("→0x%x", v.Pointer())
	case reflect.Chan, reflect.Func:
		if v.IsNil() {
			return "nil"
		}
		return fmt.Sprintf("0x%x", v.Pointer())
	case reflect.Map:
		if v.IsNil() {
			return "nil map"
		}
		return fmt.Sprintf("map, len %d", v.Len())
	case reflect.Slice:
		if v.IsNil() {
			return "nil срез"
		}
		return seqPreview(v, depth)
	case reflect.Array:
		return seqPreview(v, depth)
	case reflect.Interface:
		if v.IsNil() {
			return "nil"
		}
		return typeName(v.Elem().Type()) + "(" + fmtVal(v.Elem(), depth+1) + ")"
	case reflect.Struct:
		if depth >= 2 {
			return "{…}"
		}
		var parts []string
		for i := 0; i < v.NumField() && i < 4; i++ {
			parts = append(parts, fmtVal(v.Field(i), depth+1))
		}
		if v.NumField() > 4 {
			parts = append(parts, "…")
		}
		return "{" + strings.Join(parts, " ") + "}"
	}
	return v.Kind().String()
}

func seqPreview(v reflect.Value, depth int) string {
	if depth >= 2 {
		return "[…]"
	}
	n := v.Len()
	var parts []string
	for i := 0; i < n && i < 6; i++ {
		parts = append(parts, fmtVal(v.Index(i), depth+1))
	}
	if n > 6 {
		parts = append(parts, "…")
	}
	return "[" + strings.Join(parts, " ") + "]"
}
