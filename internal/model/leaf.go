package model

import (
	"fmt"
	"github.com/vitikevich-landau/go_magic_eye/internal/text"
	"reflect"
	"unsafe"
)

// leaf — регион для не-структурного поля (или не-структурного корня).
// Здесь живут все уроки: little-endian, заголовки string/slice/map,
// два слова интерфейса, честные отказы.
func (b *builder) leaf(t reflect.Type, v reflect.Value, off uintptr, name, from string) {
	r := Region{Kind: RField, Offset: off, Size: t.Size(),
		Name: rootName(name), TypeName: typeName(t), From: from}
	hasV := b.val(v)
	if hasV {
		v = readable(v)
	}

	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if hasV {
			r.Value = fmt.Sprintf("%d (0x%x)", v.Int(), uint64(v.Int())&mask(t.Size()))
			b.teachLE(&r, t)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if hasV {
			r.Value = fmt.Sprintf("%d (0x%x)", v.Uint(), v.Uint())
			b.teachLE(&r, t)
		}
	case reflect.Uintptr:
		if hasV {
			r.Value = fmt.Sprintf("0x%x", v.Uint())
		}
		r.Note = "голое число-адрес: GC его НЕ видит — объект по нему не удержится"
	case reflect.Bool:
		if hasV {
			r.Value = fmt.Sprint(v.Bool())
		}
	case reflect.Float32, reflect.Float64:
		if hasV {
			r.Value = fmt.Sprint(v.Float())
			r.Note = "IEEE-754: знак · экспонента · мантисса (см. hex)"
		}
	case reflect.Complex64, reflect.Complex128:
		if hasV {
			r.Value = fmt.Sprint(v.Complex())
		}
		r.Note = "две вещественные половины подряд: re, im"

	case reflect.String:
		r.Note = fmt.Sprintf("заголовок строки, %d слова: data *byte + len int", 2)
		if hasV {
			s := v.String()
			r.Value = fmt.Sprintf("%s · len %d · data 0x%x", quote(s, 16), len(s), strData(s))
			b.stringSat(r.Name, s)
		}
	case reflect.Slice:
		r.Note = "заголовок среза, 3 слова: data + len + cap"
		if hasV {
			if v.IsNil() {
				r.Value = "nil (все три слова нулевые)"
			} else {
				r.Value = fmt.Sprintf("len %d · cap %d · data 0x%x", v.Len(), v.Cap(), v.Pointer())
				b.sliceSat(r.Name, v)
			}
		}
	case reflect.Array:
		r.Note = fmt.Sprintf("массив ЦЕЛИКОМ в объекте: %d × %s (не срез, заголовка нет)",
			t.Len(), typeName(t.Elem()))
		if hasV {
			r.Value = fmtVal(v, 0)
		}
	case reflect.Map:
		r.Note = "одно слово: *runtime.hmap " + text.Rune("→", "->") + " снаружи бакеты по 8 ячеек (см. спутник)"
		if hasV {
			if v.IsNil() {
				r.Value = "nil map (читать можно, писать — panic)"
			} else {
				r.Value = fmt.Sprintf("len %d · *hmap 0x%x", v.Len(), v.Pointer())
				b.mapSat(r.Name, v)
			}
		}
	case reflect.Chan:
		r.Note = "одно слово: *runtime.hchan " + text.Rune("→", "->") + " кольцевой буфер + очереди горутин"
		if hasV {
			if v.IsNil() {
				r.Value = "nil chan (приём/передача заблокируются навсегда)"
			} else {
				r.Value = fmt.Sprintf("len %d / cap %d · *hchan 0x%x", v.Len(), v.Cap(), v.Pointer())
			}
		}
	case reflect.Func:
		r.Note = "одно слово: *funcval (замыкание: код + захваченные переменные)"
		if hasV {
			if v.IsNil() {
				r.Value = "nil func"
			} else {
				r.Value = fmt.Sprintf("0x%x %s %s", v.Pointer(), text.Rune("→", "->"), funcName(v.Pointer()))
			}
		}

	case reflect.Pointer:
		if hasV {
			if v.IsNil() {
				r.Value = "nil"
			} else {
				r.Value = fmt.Sprintf("%s 0x%x", text.Rune("→", "->"), v.Pointer())
				b.ptrSat(r.Name, v)
			}
		}
	case reflect.UnsafePointer:
		r.Note = "тип стёрт: что за адресом — неизвестно, Око по нему не пойдёт"
		if hasV {
			if v.IsNil() {
				r.Value = "nil"
			} else {
				r.Value = fmt.Sprintf("%s 0x%x (?)", text.Rune("→", "->"), v.Pointer())
			}
		}

	case reflect.Interface:
		if t.NumMethod() == 0 {
			r.Note = "eface, 2 слова: *_type (динамический тип) + data"
		} else {
			r.Note = "iface, 2 слова: *itab (тип+таблица методов) + data — «vtable» Go живёт ЗДЕСЬ, в значении, а не в объекте"
		}
		if hasV {
			r.Value = b.ifaceLeaf(t, v, r.Name)
		}

	default:
		if hasV {
			r.Value = fmtVal(v, 0)
		}
	}
	b.region(r)
}

// mask — маска значащих байт: hex-представление int8(-2) должно показать
// 0xfe, а не 0xfffffffffffffffe (знаковое расширение до int64 при v.Int()).
func mask(size uintptr) uint64 {
	if size >= 8 {
		return ^uint64(0)
	}
	return 1<<(8*size) - 1
}

// littleEndian — порядок байт ЭТОЙ машины, определённый честно, в рантайме:
// кладём uint16(1) и смотрим, какой байт лёг первым. Почти всюду Go живёт на
// little-endian (amd64/arm64/386/riscv), но s390x, mips и ppc64 — big-endian,
// и врать про них урок не должен.
var littleEndian = func() bool {
	x := uint16(1)
	return *(*byte)(unsafe.Pointer(&x)) == 1
}()

// teachLE — урок порядка байт: один раз, на первом многобайтовом целом.
func (b *builder) teachLE(r *Region, t reflect.Type) {
	if b.leTaught || t.Size() < 2 {
		return
	}
	b.leTaught = true
	if littleEndian {
		r.Note = "little-endian: в дампе младший байт стоит ПЕРВЫМ — читай hex справа налево"
	} else {
		r.Note = "big-endian (экзотика: s390x/mips/ppc64): старший байт первым — hex читается слева направо"
	}
}

// strData — адрес байтового буфера строки (слово data её заголовка).
// У пустой строки буфера может не быть вовсе — честный 0.
func strData(s string) uintptr {
	if len(s) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(unsafe.StringData(s)))
}

// ── спутники: память вне объекта ────────────────────────────────────────────
//
// В самом объекте лежат только ЗАГОЛОВКИ (data+len, data+len+cap, *hmap).
// Настоящее содержимое живёт где-то ещё — в .rodata или куче. Спутник — это
// панель «а вот что по ту сторону указателя»: байты берутся unsafe.Slice
// прямо поверх живой памяти, без копий (потому и контракт: объект должен
// жить, пока Око смотрит).

const satElems = 16 // элементов в статическом превью спутника

func (b *builder) stringSat(name, s string) {
	if len(s) == 0 {
		return
	}
	sat := Satellite{
		Title: fmt.Sprintf("буфер строки «%s»", name),
		Addr:  strData(s), Size: uintptr(len(s)),
		Bytes: unsafe.Slice(unsafe.StringData(s), len(s)),
		Note: "строка неизменяема; литералы живут в .rodata (это НЕ куча), " +
			"куча появляется при конкатенации/строительстве",
	}
	b.m.Sats = append(b.m.Sats, sat)
}

func (b *builder) sliceSat(name string, v reflect.Value) {
	n := v.Len()
	sat := Satellite{
		Title: fmt.Sprintf("хребет среза «%s»", name),
		Addr:  v.Pointer(), Size: uintptr(v.Cap()) * v.Type().Elem().Size(),
	}
	for i := 0; i < n && i < satElems; i++ {
		sat.Elems = append(sat.Elems, fmt.Sprintf("[%d] %s", i, fmtVal(v.Index(i), 1)))
	}
	if n > satElems {
		sat.Elems = append(sat.Elems, fmt.Sprintf("%s ещё %d элем. (странствие покажет постранично)", text.Rune("⋯", "..."), n-satElems))
	}
	if extra := v.Cap() - n; extra > 0 {
		sat.Note = fmt.Sprintf("за len прячется cap-хвост: ещё %d слотов уже выделено — append туда, без реаллокации", extra)
	}
	b.m.Sats = append(b.m.Sats, sat)
}

func (b *builder) mapSat(name string, v reflect.Value) {
	sat := Satellite{
		Title: fmt.Sprintf("содержимое map «%s»", name),
		Addr:  v.Pointer(), Size: 0,
		Note: "порядок обхода map в Go НАРОЧНО случаен (рантайм подмешивает seed) — не полагайся на него",
	}
	it := v.MapRange()
	i := 0
	for it.Next() && i < satElems {
		sat.Elems = append(sat.Elems, fmt.Sprintf("%s %s %s", fmtVal(it.Key(), 1), text.Rune("→", "->"), fmtVal(it.Value(), 1)))
		i++
	}
	if v.Len() > satElems {
		sat.Elems = append(sat.Elems, fmt.Sprintf("%s ещё %d пар", text.Rune("⋯", "..."), v.Len()-satElems))
	}
	b.m.Sats = append(b.m.Sats, sat)
}

// ptrSat — цель указателя: скаляр или маленькая структура — покажем байты.
func (b *builder) ptrSat(name string, v reflect.Value) {
	et := v.Type().Elem()
	if et.Size() == 0 || et.Size() > 64 || et.Kind() == reflect.UnsafePointer {
		return
	}
	target := v.Elem()
	sat := Satellite{
		Title: fmt.Sprintf("цель «%s» — %s", name, typeName(et)),
		Addr:  v.Pointer(), Size: et.Size(),
		Bytes: unsafe.Slice((*byte)(v.UnsafePointer()), et.Size()),
		Note:  "значение: " + fmtVal(target, 0),
	}
	b.m.Sats = append(b.m.Sats, sat)
}

// ── интерфейсное поле: значение + запись в секцию интерфейсов ───────────────

func (b *builder) ifaceLeaf(t reflect.Type, v reflect.Value, name string) string {
	info := b.ifaceInfo(t, v, name)
	b.m.Ifaces = append(b.m.Ifaces, info)
	if v.IsNil() {
		return "nil (оба слова нулевые)"
	}
	val := fmt.Sprintf("тип %s · tab 0x%x · data 0x%x",
		info.DynType, info.TabAddr, info.DataAddr)
	if info.TypedNil {
		val += " " + text.Rune("←", "<-") + " ЛОВУШКА: typed nil"
	}
	return val
}
