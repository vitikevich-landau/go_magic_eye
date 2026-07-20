// Package diag — перевод вывода `go build -gcflags=-e` в диагностики с
// позициями для маркеров редактора. Флаг -e снимает лимит «10 ошибок и
// молчок»: ученику показывают всё.
package diag

import (
	"regexp"
	"strconv"
	"strings"
)

// Diag — одна диагностика компилятора, адресованная редактору.
type Diag struct {
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	Severity string `json:"severity"` // пока всегда "error": go build о стиле не ворчит
	Message  string `json:"message"`
}

// Строка ошибки: `./main.go:12:7: undefined: knight` (колонки может не быть).
// Имя файла в снипетте всегда main.go — пользовательский код уходит в сборку
// одним файлом без обёрток, поэтому позиции не требуют сдвига.
var lineRe = regexp.MustCompile(`^(?:\./)?main\.go:(\d+)(?::(\d+))?: (.*)$`)

// Parse — диагностики из stderr компилятора. Строки-продолжения (с отступом,
// например «have …/want …» у несовпавших сигнатур) приклеиваются к последней
// диагностике: это части одного урока.
func Parse(out string) []Diag {
	diags := []Diag{}
	for _, raw := range strings.Split(out, "\n") {
		if strings.HasPrefix(raw, "# ") || strings.TrimSpace(raw) == "" {
			continue // заголовок пакета от go build — не диагностика
		}
		if m := lineRe.FindStringSubmatch(raw); m != nil {
			line, _ := strconv.Atoi(m[1])
			col := 1
			if m[2] != "" {
				col, _ = strconv.Atoi(m[2])
			}
			diags = append(diags, Diag{Line: line, Col: col, Severity: "error", Message: m[3]})
			continue
		}
		if (strings.HasPrefix(raw, "\t") || strings.HasPrefix(raw, "    ")) && len(diags) > 0 {
			diags[len(diags)-1].Message += "\n" + strings.TrimSpace(raw)
		}
		// прочие строки (сбой toolchain и т.п.) — не позиционные: их отдаёт
		// верхний уровень как stderr целиком, здесь они не нужны
	}
	return diags
}
