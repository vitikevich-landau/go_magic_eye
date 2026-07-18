package model

import (
	"fmt"
	"github.com/vitikevich-landau/go_magic_eye/internal/text"
	"reflect"
	"sort"
	"strings"
	"unsafe"
)

// wordSize — машинное слово платформы: 8 на 64-битных, 4 на 32-битных.
// Через него считаются все «слова» заголовков (string, slice, interface).
const wordSize = unsafe.Sizeof(uintptr(0))

// Of — полный осмотр объекта, переданного через any.
//
//   - дали указатель → смотрим на ОРИГИНАЛ по живому адресу (без копии);
//   - дали значение → оно уже упаковано в interface: смотрим на коробку
//     (это копия — и это честно записано в заметках; сама упаковка — урок).
func Of(obj any, label string) *Model {
	rv := reflect.ValueOf(obj)
	if !rv.IsValid() {
		return &Model{
			Label:    label,
			Passport: Passport{TypeName: "nil", Kind: "пустой interface: оба слова нулевые"},
			Notes:    []string{"Inspect(nil): в eface нет ни типа, ни данных — смотреть не на что."},
		}
	}
	if rv.Kind() == reflect.Pointer && !rv.IsNil() {
		// один уровень разыменования: Inspect(&x) смотрит на x по живому
		// адресу, какой бы x ни была — хоть сама указателем (**T)
		m := OfValue(rv.Elem(), label)
		m.Notes = append(m.Notes, fmt.Sprintf(
			"принят указатель %s — Око смотрит на оригинал по адресу 0x%x, копий нет.",
			typeName(rv.Type()), m.Addr))
		return m
	}
	box := reflect.New(rv.Type())
	box.Elem().Set(rv)
	m := OfValue(box.Elem(), label)
	m.Notes = append(m.Notes, fmt.Sprintf(
		"значение упаковано в any (interface{}): Око смотрит на коробку интерфейса — копию. "+
			"Хочешь оригинал на месте — передай &объект. Указатели внутри копии ведут к тем же данным."))
	return m
}

// OfValue — осмотр адресуемого reflect.Value (живой памяти).
func OfValue(v reflect.Value, label string) *Model {
	t := v.Type()
	b := &builder{m: &Model{Label: label, HasValue: true}}
	b.m.Passport = passportOf(t)
	if v.CanAddr() {
		p := v.Addr().UnsafePointer()
		b.m.Addr = uintptr(p)
		if t.Size() > 0 {
			b.m.Bytes = unsafe.Slice((*byte)(p), t.Size())
		}
	}
	b.walk(t, v, 0, "", "", 0)
	b.finish(t)
	return b.m
}

// OfType — только статика типа: объекта нет, значения и байты пусты.
func OfType(t reflect.Type, label string) *Model {
	b := &builder{m: &Model{Label: label, HasValue: false}}
	b.m.Passport = passportOf(t)
	b.walk(t, reflect.Value{}, 0, "", "", 0)
	b.finish(t)
	return b.m
}

// passportOf — статика типа. Всё берётся из reflect.Type, объект не нужен:
// размер и выравнивание компилятор знает на этапе сборки, comparable-ность —
// свойство типа (slice/map/func делают тип несравнимым), а «видит ли GC» —
// вопрос «есть ли в типе хоть одно слово-указатель».
func passportOf(t reflect.Type) Passport {
	p := Passport{
		TypeName: typeName(t),
		Kind:     kindName(t),
		Size:     t.Size(),
		Align:    uintptr(t.Align()),
	}
	if t.Comparable() {
		p.Traits = append(p.Traits, "comparable (можно ==, ключ map)")
	} else {
		p.Traits = append(p.Traits, "НЕ comparable (== не скомпилируется)")
	}
	if hasPointers(t, 0) {
		p.Traits = append(p.Traits, "содержит указатели "+text.Rune("→", "->")+" GC сканирует")
	} else {
		p.Traits = append(p.Traits, "без указателей "+text.Rune("→", "->")+" GC пропускает целиком")
	}
	if t.Size() == 0 {
		p.Traits = append(p.Traits, "нулевой размер (все такие объекты могут делить адрес)")
	}
	if t.Kind() == reflect.Struct || t.Kind() == reflect.Pointer {
		n := reflect.PointerTo(t).NumMethod()
		if t.Kind() == reflect.Pointer {
			n = t.NumMethod()
		}
		if n > 0 {
			p.Traits = append(p.Traits, fmt.Sprintf("методов в method set *T: %d", n))
		}
	}
	return p
}

// hasPointers — есть ли в типе слова, интересные GC.
//
// Урок: сборщик мусора сканирует объект только если в его типе есть хотя бы
// одно слово-указатель (в рантайме это решают ptrdata/gcdata типа). Структура
// из одних чисел — «без указателей»: GC пропускает её целиком, сколько бы
// мегабайт она ни занимала. Поэтому []struct{X,Y float64} дешевле для GC, чем
// []*Point — это один из главных практических выводов всей карты памяти.
func hasPointers(t reflect.Type, depth int) bool {
	if depth > 10 {
		return true
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.UnsafePointer, reflect.Map, reflect.Chan,
		reflect.Func, reflect.Slice, reflect.String, reflect.Interface:
		return true
	case reflect.Array:
		return t.Len() > 0 && hasPointers(t.Elem(), depth+1)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if hasPointers(t.Field(i).Type, depth+1) {
				return true
			}
		}
	}
	return false
}

// ── строитель ────────────────────────────────────────────────────────────────

type builder struct {
	m        *Model
	leTaught bool // урок little-endian даём один раз
}

func (b *builder) val(v reflect.Value) bool { return b.m.HasValue && v.IsValid() }

// walk раскладывает значение типа t по регионам, начиная с offset off.
func (b *builder) walk(t reflect.Type, v reflect.Value, off uintptr, path, from string, depth int) {
	if t.Kind() == reflect.Struct {
		b.walkStruct(t, v, off, path, from, depth)
		return
	}
	b.leaf(t, v, off, path, from)
}

// walkStruct — сердце карты памяти. Идём по полям в порядке объявления
// (Go никогда не переставляет поля — раскладка в руках автора) и держим
// «конец предыдущего поля» (end): разрыв между end и offset очередного поля —
// это padding-дыра, и мы знаем её причину — выравнивание следующего поля.
//
//	f.Offset   — смещение поля внутри t (посчитал компилятор);
//	off        — смещение самого t внутри КОРНЕВОГО объекта: при рекурсии
//	             во вложенные/встроенные структуры смещения складываются,
//	             поэтому вся карта — в координатах корня.
func (b *builder) walkStruct(t reflect.Type, v reflect.Value, off uintptr, path, from string, depth int) {
	if depth > 6 {
		b.region(Region{Kind: RField, Offset: off, Size: t.Size(),
			Name: path + "…", TypeName: typeName(t),
			Note: "глубина вложенности >6 — дальше Око не разворачивает (странствие может)"})
		return
	}
	end := off
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fo := off + f.Offset
		b.gap(end, fo, "поле «"+f.Name+"» ("+typeName(f.Type)+") требует адрес, кратный "+
			fmt.Sprint(f.Type.Align()))
		var fv reflect.Value
		if b.val(v) {
			fv = v.Field(i)
		}
		name := path + f.Name
		switch {
		case f.Anonymous && f.Type.Kind() == reflect.Struct:
			// встраивание («наследование»): под-объект + рекурсия по его
			// полям; from помечает, из какого типа поле пришло
			b.embed(t, f, fo, depth)
			b.walkStruct(f.Type, fv, fo, path, typeName(f.Type), depth+1)
		case f.Anonymous && f.Type.Kind() == reflect.Pointer:
			// встроен УКАЗАТЕЛЬ (struct{ *Base }): в объекте лежит одно
			// слово-ссылка, никакого под-объекта — честно рисуем указатель
			b.embed(t, f, fo, depth)
			b.leaf(f.Type, fv, fo, name, from)
		case f.Type.Kind() == reflect.Struct:
			// обычное поле-структура: разворачиваем с префиксом «pos.»
			b.walkStruct(f.Type, fv, fo, name+".", from, depth+1)
		default:
			b.leaf(f.Type, fv, fo, name, from)
		}
		end = fo + f.Type.Size()
	}
	// Хвостовая дыра: размер структуры всегда кратен её выравниванию — иначе
	// в массиве []T второй элемент оказался бы невыровнен. (В Go, в отличие
	// от C++, хвост встроенной структуры НЕ переиспользуется под соседей.)
	b.gap(end, off+t.Size(),
		fmt.Sprintf("хвост структуры: размер добит до кратного align (%d)", t.Align()))
}

// gap — padding-дыра между last и next (если есть).
func (b *builder) gap(last, next uintptr, why string) {
	if next > last {
		b.region(Region{Kind: RPadding, Offset: last, Size: next - last,
			Name: "padding", Note: why})
	}
}

func (b *builder) embed(outer reflect.Type, f reflect.StructField, off uintptr, depth int) {
	e := Embed{
		Depth: depth, TypeName: typeName(f.Type), FieldName: f.Name,
		Offset: off, Size: f.Type.Size(),
	}
	e.Promoted = promoted(outer, f.Type)
	if f.Type.Kind() == reflect.Pointer {
		e.Note = "встроен УКАЗАТЕЛЬ: под-объекта внутри нет, только слово-ссылка (аналога в C++ нет)"
	}
	b.m.Embeds = append(b.m.Embeds, e)
}

// promoted — методы встроенного типа, продвинутые во внешний.
//
// Правило языка: методы анонимного поля входят в method set обёртки (если
// имя не затенено). Проверяем прямо: берём методы *inner и оставляем те,
// что находятся и у *outer. Сравниваем по *T — его method set самый полный
// (включает методы и значения, и указателя).
func promoted(outer, inner reflect.Type) []string {
	op := reflect.PointerTo(outer)
	ip := inner
	if inner.Kind() != reflect.Pointer {
		ip = reflect.PointerTo(inner)
	}
	var out []string
	for i := 0; i < ip.NumMethod(); i++ {
		name := ip.Method(i).Name
		if _, ok := op.MethodByName(name); ok {
			out = append(out, name)
		}
	}
	return out
}

func (b *builder) region(r Region) { b.m.Regions = append(b.m.Regions, r) }

// finish — сортировка, советы, общий порядок.
func (b *builder) finish(t reflect.Type) {
	sort.SliceStable(b.m.Regions, func(i, j int) bool {
		return b.m.Regions[i].Offset < b.m.Regions[j].Offset
	})
	var holes uintptr
	for _, r := range b.m.Regions {
		if r.Kind == RPadding {
			holes += r.Size
		}
	}
	if holes > 0 {
		note := fmt.Sprintf("в объекте %d Б дыр (Go порядок полей НЕ переставляет — раскладка в руках автора).", holes)
		// совет с ПЕРЕСЧЁТОМ: не «отсортируй и станет лучше», а сколько
		// именно байт даст перестановка по убыванию выравнивания
		if t.Kind() == reflect.Struct {
			if opt := optimalSize(t); opt < t.Size() {
				note = fmt.Sprintf(
					"в объекте %d Б дыр: перестановка полей по убыванию выравнивания даст %d Б вместо %d "+
						"(экономия %d Б; Go сам поля не переставляет).",
					holes, opt, t.Size(), t.Size()-opt)
			}
		}
		b.m.Notes = append(b.m.Notes, note)
	}
}

// optimalSize — размер структуры при «жадной» раскладке: поля по убыванию
// выравнивания (при равном — по убыванию размера). Для плоских структур это
// минимум padding'а; учебная оценка — вложенные структуры не пересобираются.
func optimalSize(t reflect.Type) uintptr {
	n := t.NumField()
	if n == 0 {
		return t.Size()
	}
	fs := make([]reflect.StructField, n)
	for i := range fs {
		fs[i] = t.Field(i)
	}
	sort.SliceStable(fs, func(i, j int) bool {
		ai, aj := fs[i].Type.Align(), fs[j].Type.Align()
		if ai != aj {
			return ai > aj
		}
		return fs[i].Type.Size() > fs[j].Type.Size()
	})
	var off, maxA uintptr = 0, 1
	for _, f := range fs {
		a := uintptr(f.Type.Align())
		if a == 0 {
			a = 1
		}
		if a > maxA {
			maxA = a
		}
		off = (off + a - 1) / a * a // выровнять начало поля
		off += f.Type.Size()
	}
	return (off + maxA - 1) / maxA * maxA // хвост до кратного align структуры
}

// pathless — имя для выносок безымянных корней.
func rootName(path string) string {
	if path == "" {
		return "значение"
	}
	return strings.TrimSuffix(path, ".")
}
