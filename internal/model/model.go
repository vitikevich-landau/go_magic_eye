// Package model — МОДЕЛЬ Ока: превращает живой объект Go в плоские структуры
// данных, которые затем рисует вид (internal/render) и обходит странствие
// (internal/nav).
//
// Это шов №1 проекта: вид не знает о reflect/unsafe, модель не знает о
// рамках и цветах. Всё, что модель добыла, лежит в Model.
package model

// Passport — статика типа: то, что известно без объекта.
type Passport struct {
	TypeName string
	Kind     string   // reflect.Kind человеческим словом
	Size     uintptr  // unsafe.Sizeof
	Align    uintptr  // unsafe.Alignof
	Traits   []string // comparable, «содержит указатели (GC сканирует)», …
}

// RegionKind — сорт региона карты памяти.
type RegionKind int

const (
	RField   RegionKind = iota // поле (или машинное слово заголовка)
	RPadding                   // дыра выравнивания
	RWord                      // служебное слово (data/len/cap, itab/data)
)

// Region — непрерывный кусок памяти объекта с выноской.
type Region struct {
	Kind     RegionKind
	Offset   uintptr
	Size     uintptr
	Name     string // «hp», «pos.x», «(хвост)»
	TypeName string
	Value    string // строковое значение («100 (0x64)», «"Griffin…" len 7»)
	Note     string // урок: little-endian, причина дыры, устройство заголовка
	From     string // тип встроенной структуры, из которой поле пришло
}

// Embed — под-объект встроенной структуры (аналог под-объекта базы в C++).
type Embed struct {
	Depth     int
	TypeName  string
	FieldName string
	Offset    uintptr
	Size      uintptr
	Promoted  []string // методы, продвинутые наружу
	Note      string
}

// Method — строка «vtable»-диаграммы: слот itab.
type Method struct {
	Name string  // имя метода в интерфейсе
	PC   uintptr // адрес из itab.fun[i] (0 — не читали)
	Func string  // имя функции по runtime.FuncForPC
}

// Iface — анатомия одного interface-значения (корня или поля).
type Iface struct {
	Where    string // «объект целиком» или имя поля
	Empty    bool   // any/eface (без методов) или iface (с itab)
	TypeName string // статический тип интерфейса
	DynType  string // динамический тип («⌀ nil» если пуст)
	TabAddr  uintptr
	DataAddr uintptr
	Hash     uint32
	Methods  []Method
	TypedNil bool // ловушка: интерфейс НЕ nil, а указатель внутри — nil
	Note     string
}

// Satellite — панель-спутник: память, живущая вне объекта (буфер строки,
// хребет среза, цель указателя).
type Satellite struct {
	Title string
	Addr  uintptr
	Size  uintptr
	Bytes []byte   // сырые байты (строки, цели указателей)
	Elems []string // элементы (срезы, массивы, map)
	Note  string
}

// Model — всё, что Око увидело в одном объекте.
type Model struct {
	Label    string
	Passport Passport
	HasValue bool    // false для InspectType[T]() — только статика
	Addr     uintptr // адрес объекта (коробки интерфейса)
	Bytes    []byte  // сырые байты объекта (живая память, не копия)
	Regions  []Region
	Embeds   []Embed
	Ifaces   []Iface
	Sats     []Satellite
	Notes    []string // общие заметки/советы (перестановка полей и т.п.)
}
