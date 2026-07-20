package sandbox

import (
	"bytes"
	"encoding/json"
)

// envelopeMark — начало конверта, каким его печатает библиотека
// (MarshalIndent с двумя пробелами): по нему находится конверт, даже
// приклеенный к чужому хвосту без перевода строки.
var envelopeMark = []byte("{\n  \"eye_json_version\"")

// ExtractEnvelopes — выуживает из смешанного stdout программы конверты Ока
// (их может быть несколько: каждый Inspect печатает свой) и сливает в один
// {"eye_json_version":1,"models":[…]}. Остаток — пользовательский вывод
// (fmt.Println — законная часть обучения), он возвращается как rest.
//
// Кандидат на конверт — «{» в начале вывода или сразу после перевода строки;
// подтверждение — успешный разбор JSON-объекта с eye_json_version == 1.
// Пользовательский «{...}» без этого поля конвертом не считается.
func ExtractEnvelopes(out []byte) (envelope []byte, rest string) {
	type env struct {
		Version int             `json:"eye_json_version"`
		Models  json.RawMessage `json:"models"`
	}
	var models []json.RawMessage
	var restBuf bytes.Buffer

	for pos := 0; pos < len(out); {
		// следующий кандидат: «{» прямо на текущей позиции (начало вывода
		// или стык двух конвертов), первый «{» после перевода строки, а
		// также конверт, приклеенный к хвосту без \n (fmt.Print перед
		// Inspect) — его выдаёт сигнатура отступного MarshalIndent
		cand := -1
		if out[pos] == '{' {
			cand = pos
		} else if i := bytes.Index(out[pos:], []byte("\n{")); i >= 0 {
			cand = pos + i + 1
		}
		if i := bytes.Index(out[pos:], envelopeMark); i >= 0 && (cand < 0 || pos+i < cand) {
			cand = pos + i
		}
		if cand < 0 {
			restBuf.Write(out[pos:])
			break
		}
		dec := json.NewDecoder(bytes.NewReader(out[cand:]))
		var e env
		var ms []json.RawMessage
		// конверт — только ПОЛНАЯ форма: версия плюс массив models (пустой
		// законен). Лог-самозванец {"eye_json_version":1} без models — печать
		// пользователя, съесть её молча нельзя (тот же принцип, что у
		// isProtocolLine в сеансах: половинных форм контракт не знает)
		if err := dec.Decode(&e); err == nil && e.Version == 1 &&
			bytes.HasPrefix(bytes.TrimSpace(e.Models), []byte("[")) &&
			json.Unmarshal(e.Models, &ms) == nil {
			// один \n перед конвертом — разделитель самой библиотеки
			// (конверт начинается с чистой строки), не печать пользователя:
			// глотаем ровно его, пользовательские переводы строк целы
			prefix := out[pos:cand]
			prefix = bytes.TrimSuffix(prefix, []byte("\n"))
			restBuf.Write(prefix)
			models = append(models, ms...)
			pos = cand + int(dec.InputOffset())
			// съесть перевод строки, оставшийся от печати конверта
			if pos < len(out) && out[pos] == '\n' {
				pos++
			}
			continue
		}
		// не конверт: «{» уходит в остаток, поиск продолжается за ним
		restBuf.Write(out[pos : cand+1])
		pos = cand + 1
	}

	if models == nil {
		return nil, restBuf.String()
	}
	merged, err := json.Marshal(struct {
		Version int               `json:"eye_json_version"`
		Models  []json.RawMessage `json:"models"`
	}{1, models})
	if err != nil {
		return nil, restBuf.String()
	}
	return merged, restBuf.String()
}
