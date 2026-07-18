package model

import (
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

type base struct {
	hp int32
}

func (b *base) Walk() string { return "walk" }

type hero struct {
	base
	armor  int16
	banner string
	loot   []int64
	ledger map[string]int
	buddy  *hero
}

func (h *hero) Speak() string { return "hi" }
func (h *hero) Volume() int   { return 1 }

type speaker interface {
	Speak() string
	Volume() int
}

func heroModel(t *testing.T) (*hero, *Model) {
	t.Helper()
	h := &hero{armor: 30, banner: "Griffin", loot: []int64{1, 2, 3},
		ledger: map[string]int{"a": 1}}
	h.hp = 100
	h.buddy = h
	return h, Of(h, "тест")
}

func TestPassport(t *testing.T) {
	_, m := heroModel(t)
	if m.Passport.Size != unsafe.Sizeof(hero{}) {
		t.Fatalf("size: %d != %d", m.Passport.Size, unsafe.Sizeof(hero{}))
	}
	if m.Passport.Align != unsafe.Alignof(hero{}) {
		t.Fatalf("align: %d", m.Passport.Align)
	}
	joined := strings.Join(m.Passport.Traits, "; ")
	if !strings.Contains(joined, "НЕ comparable") {
		t.Fatalf("hero со срезом должен быть НЕ comparable: %q", joined)
	}
}

func TestPointerRootIsLive(t *testing.T) {
	h, m := heroModel(t)
	if m.Addr != uintptr(unsafe.Pointer(h)) {
		t.Fatalf("Of(&h) должен смотреть на оригинал: 0x%x != %p", m.Addr, h)
	}
	if len(m.Bytes) != int(unsafe.Sizeof(*h)) {
		t.Fatalf("байты объекта: %d", len(m.Bytes))
	}
}

func TestRegionsOffsetsAndPadding(t *testing.T) {
	_, m := heroModel(t)
	byName := map[string]Region{}
	var holes uintptr
	for _, r := range m.Regions {
		if r.Kind == RPadding {
			holes += r.Size
			continue
		}
		byName[r.Name] = r
	}
	ht := reflect.TypeOf(hero{})
	af, _ := ht.FieldByName("armor")
	if r, ok := byName["armor"]; !ok || r.Offset != af.Offset {
		t.Fatalf("armor offset: %+v vs %d", byName["armor"], af.Offset)
	}
	// hp пришло из встроенной base и должно быть помечено
	if r, ok := byName["hp"]; !ok || r.From == "" {
		t.Fatalf("hp должен нести пометку встраивания: %+v", r)
	}
	// int32+int16 в начале → дыра до string обязана существовать
	if holes == 0 {
		t.Fatal("в hero есть дыры выравнивания, а модель их не нашла")
	}
	// неэкспортированное значение прочитано
	if r := byName["armor"]; !strings.Contains(r.Value, "30") {
		t.Fatalf("значение armor: %q", r.Value)
	}
	if r := byName["banner"]; !strings.Contains(r.Value, "Griffin") {
		t.Fatalf("значение banner: %q", r.Value)
	}
}

func TestEmbeds(t *testing.T) {
	_, m := heroModel(t)
	if len(m.Embeds) != 1 || m.Embeds[0].TypeName != "model.base" {
		t.Fatalf("встраивание: %+v", m.Embeds)
	}
	if len(m.Embeds[0].Promoted) == 0 || m.Embeds[0].Promoted[0] != "Walk" {
		t.Fatalf("продвинутые методы: %+v", m.Embeds[0].Promoted)
	}
}

func TestSatellites(t *testing.T) {
	_, m := heroModel(t)
	var titles []string
	for _, s := range m.Sats {
		titles = append(titles, s.Title)
	}
	joined := strings.Join(titles, "; ")
	for _, want := range []string{"banner", "loot", "ledger"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("нет спутника %q: %v", want, titles)
		}
	}
	// буфер строки — живые байты
	for _, s := range m.Sats {
		if strings.Contains(s.Title, "banner") && string(s.Bytes) != "Griffin" {
			t.Fatalf("байты строки: %q", s.Bytes)
		}
	}
}

func TestIfaceAnatomy(t *testing.T) {
	h := &hero{}
	var s speaker = h
	m := Of(&s, "iface")
	if len(m.Ifaces) != 1 {
		t.Fatalf("ифейсов: %d", len(m.Ifaces))
	}
	info := m.Ifaces[0]
	if info.Empty || info.DynType != "*model.hero" {
		t.Fatalf("динамический тип: %+v", info)
	}
	if len(info.Methods) != 2 || info.Methods[0].Name != "Speak" || info.Methods[1].Name != "Volume" {
		t.Fatalf("методы: %+v", info.Methods)
	}
	if uintptr(unsafe.Pointer(h)) != info.DataAddr {
		t.Fatalf("data должен указывать на объект: 0x%x", info.DataAddr)
	}
	// сырые слоты itab: на 64-битных платформах имена обязаны разрешиться
	if wordSize == 8 {
		for _, mt := range info.Methods {
			if mt.PC == 0 || !strings.Contains(mt.Func, "hero") {
				t.Fatalf("слот %s не разрешился: %+v", mt.Name, mt)
			}
		}
	}
}

func TestTypedNil(t *testing.T) {
	var ghost *hero
	var s speaker = ghost
	m := Of(&s, "ловушка")
	if len(m.Ifaces) != 1 || !m.Ifaces[0].TypedNil {
		t.Fatalf("typed nil не распознан: %+v", m.Ifaces)
	}
}

func TestOfTypeHasNoValues(t *testing.T) {
	m := OfType(reflect.TypeOf(hero{}), "тип")
	if m.HasValue || len(m.Bytes) != 0 {
		t.Fatal("OfType не должен нести значения")
	}
	for _, r := range m.Regions {
		if r.Value != "" {
			t.Fatalf("регион с значением в OfType: %+v", r)
		}
	}
}

// direct-iface: struct{p *T} хранится в слове data ПРЯМО (без коробки).
// Раньше DynDataValue принимал этот указатель за адрес структуры и читал
// чужую память.
func TestDynDataValueDirectIface(t *testing.T) {
	type wrap struct{ p *hero }
	h := &hero{armor: 7}
	var box any = wrap{p: h}
	v := reflect.ValueOf(&box).Elem()
	dyn, how, ok := DynDataValue(v)
	if !ok {
		t.Fatal("DynDataValue не дал значение")
	}
	if !strings.Contains(how, "direct-iface") {
		t.Fatalf("struct{*T} должен распознаться как direct-iface: %q", how)
	}
	got := dyn.Field(0)
	if got.Pointer() != uintptr(unsafe.Pointer(h)) {
		t.Fatalf("указатель внутри разъехался: 0x%x vs %p", got.Pointer(), h)
	}
	// не-direct структура (два поля) идёт через NewAt по адресу коробки
	type fat struct{ a, b *hero }
	box = fat{a: h, b: h}
	dyn, how, ok = DynDataValue(reflect.ValueOf(&box).Elem())
	if !ok || strings.Contains(how, "direct-iface") {
		t.Fatalf("fat не direct-iface: ok=%v how=%q", ok, how)
	}
	if dyn.Field(0).Pointer() != uintptr(unsafe.Pointer(h)) {
		t.Fatal("NewAt-путь потерял данные")
	}
}

func TestOfValueSemantics(t *testing.T) {
	x := 42
	m := Of(x, "копия")
	found := false
	for _, n := range m.Notes {
		if strings.Contains(n, "коробку интерфейса") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Of(значение) должен честно сказать про коробку: %v", m.Notes)
	}
}
