// JSON-вид модели — третий потребитель шва «модель ↔ вид» (после печати и
// странствия). Контракт зафиксирован в playground/SPEC.md §2.1 и golden-тестах:
// поля можно добавлять свободно, менять смысл существующих — только со сменой
// eye_json_version.
//
// Правила сериализации:
//   - адреса (uintptr-слова: Addr, TabAddr, PC…) — строки «0x…»: JSON-числа
//     теряют точность на 64 битах, а адрес — имя, а не величина; нулевой
//     адрес — пустая строка («адреса нет»);
//   - размеры и offset'ы — обычные числа (маленькие, ими считают);
//   - сырые байты — строка hex (фронт рисует побайтно, base64 нечитаем);
//   - пустые срезы — [], не null: потребителю не нужны null-проверки;
//   - уроки (value/note/traits) — человеческие строки, их показывают, не парсят.
package model

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// JSONVersion — версия контракта конверта.
const JSONVersion = 1

type jsonEnvelope struct {
	Version int         `json:"eye_json_version"`
	Models  []jsonModel `json:"models"`
}

type jsonModel struct {
	Label    string          `json:"label"`
	Passport jsonPassport    `json:"passport"`
	HasValue bool            `json:"has_value"`
	Addr     string          `json:"addr"`
	Bytes    string          `json:"bytes"`
	Regions  []jsonRegion    `json:"regions"`
	Embeds   []jsonEmbed     `json:"embeds"`
	Ifaces   []jsonIface     `json:"ifaces"`
	Sats     []jsonSatellite `json:"sats"`
	Notes    []string        `json:"notes"`
}

type jsonPassport struct {
	TypeName string   `json:"type_name"`
	Kind     string   `json:"kind"`
	Size     uintptr  `json:"size"`
	Align    uintptr  `json:"align"`
	Traits   []string `json:"traits"`
}

type jsonRegion struct {
	Kind     string  `json:"kind"` // "field" | "padding" | "word"
	Offset   uintptr `json:"offset"`
	Size     uintptr `json:"size"`
	Name     string  `json:"name"`
	TypeName string  `json:"type_name"`
	Value    string  `json:"value"`
	Note     string  `json:"note"`
	From     string  `json:"from"`
}

type jsonEmbed struct {
	Depth     int      `json:"depth"`
	TypeName  string   `json:"type_name"`
	FieldName string   `json:"field_name"`
	Offset    uintptr  `json:"offset"`
	Size      uintptr  `json:"size"`
	Promoted  []string `json:"promoted"`
	Note      string   `json:"note"`
}

type jsonMethod struct {
	Name string `json:"name"`
	PC   string `json:"pc"`
	Func string `json:"func"`
}

type jsonIface struct {
	Where    string       `json:"where"`
	Empty    bool         `json:"empty"`
	TypeName string       `json:"type_name"`
	DynType  string       `json:"dyn_type"`
	TabAddr  string       `json:"tab_addr"`
	DataAddr string       `json:"data_addr"`
	Hash     uint32       `json:"hash"`
	Methods  []jsonMethod `json:"methods"`
	TypedNil bool         `json:"typed_nil"`
	Note     string       `json:"note"`
}

type jsonSatellite struct {
	Title string   `json:"title"`
	Addr  string   `json:"addr"`
	Size  uintptr  `json:"size"`
	Bytes string   `json:"bytes"`
	Elems []string `json:"elems"`
	Note  string   `json:"note"`
}

// ToJSON — конверт с версией и моделями, отформатированный для чтения
// человеком (учебный инструмент: JSON будут разглядывать глазами не реже,
// чем парсить).
func ToJSON(models []*Model) ([]byte, error) {
	env := jsonEnvelope{Version: JSONVersion, Models: make([]jsonModel, 0, len(models))}
	for _, m := range models {
		env.Models = append(env.Models, toJSONModel(m))
	}
	return json.MarshalIndent(env, "", "  ")
}

func toJSONModel(m *Model) jsonModel {
	j := jsonModel{
		Label: m.Label,
		Passport: jsonPassport{
			TypeName: m.Passport.TypeName,
			Kind:     m.Passport.Kind,
			Size:     m.Passport.Size,
			Align:    m.Passport.Align,
			Traits:   strs(m.Passport.Traits),
		},
		HasValue: m.HasValue,
		Addr:     addr(m.Addr),
		Bytes:    hex.EncodeToString(m.Bytes),
		Regions:  make([]jsonRegion, 0, len(m.Regions)),
		Embeds:   make([]jsonEmbed, 0, len(m.Embeds)),
		Ifaces:   make([]jsonIface, 0, len(m.Ifaces)),
		Sats:     make([]jsonSatellite, 0, len(m.Sats)),
		Notes:    strs(m.Notes),
	}
	for _, r := range m.Regions {
		j.Regions = append(j.Regions, jsonRegion{
			Kind:     regionKind(r.Kind),
			Offset:   r.Offset,
			Size:     r.Size,
			Name:     r.Name,
			TypeName: r.TypeName,
			Value:    r.Value,
			Note:     r.Note,
			From:     r.From,
		})
	}
	for _, e := range m.Embeds {
		j.Embeds = append(j.Embeds, jsonEmbed{
			Depth:     e.Depth,
			TypeName:  e.TypeName,
			FieldName: e.FieldName,
			Offset:    e.Offset,
			Size:      e.Size,
			Promoted:  strs(e.Promoted),
			Note:      e.Note,
		})
	}
	for _, i := range m.Ifaces {
		ji := jsonIface{
			Where:    i.Where,
			Empty:    i.Empty,
			TypeName: i.TypeName,
			DynType:  i.DynType,
			TabAddr:  addr(i.TabAddr),
			DataAddr: addr(i.DataAddr),
			Hash:     i.Hash,
			Methods:  make([]jsonMethod, 0, len(i.Methods)),
			TypedNil: i.TypedNil,
			Note:     i.Note,
		}
		for _, mt := range i.Methods {
			ji.Methods = append(ji.Methods, jsonMethod{Name: mt.Name, PC: addr(mt.PC), Func: mt.Func})
		}
		j.Ifaces = append(j.Ifaces, ji)
	}
	for _, s := range m.Sats {
		j.Sats = append(j.Sats, jsonSatellite{
			Title: s.Title,
			Addr:  addr(s.Addr),
			Size:  s.Size,
			Bytes: hex.EncodeToString(s.Bytes),
			Elems: strs(s.Elems),
			Note:  s.Note,
		})
	}
	return j
}

// addr — uintptr-слово как имя: «0x…»; ноль — пустая строка («адреса нет»).
func addr(a uintptr) string {
	if a == 0 {
		return ""
	}
	return fmt.Sprintf("0x%x", a)
}

func regionKind(k RegionKind) string {
	switch k {
	case RField:
		return "field"
	case RPadding:
		return "padding"
	case RWord:
		return "word"
	}
	return "unknown"
}

// strs — nil-срез строк превращается в пустой: в JSON уйдёт [], не null.
func strs(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
