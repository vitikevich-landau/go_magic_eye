// Package examples — встроенные учебные снипетты для галереи playground.
// Адаптации examples/ основной библиотеки: каждый — самодостаточный main.go,
// который можно запустить и в терминале. Суффикс .go.txt у файлов — чтобы
// снипетты не попадали в сборку бэкенда.
package examples

import (
	"embed"
	"sort"
	"strings"
)

//go:embed snippets/*.go.txt
var snippets embed.FS

// Example — один снипетт галереи.
type Example struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Topic string `json:"topic"`
	Code  string `json:"code"`
}

// Метаданные — в шапке снипетта строками «//eye:title …» и «//eye:topic …»;
// из кода, который увидит пользователь, они вырезаются.
func parse(id string, raw []byte) Example {
	e := Example{ID: id}
	var body []string
	for _, line := range strings.Split(string(raw), "\n") {
		switch {
		case strings.HasPrefix(line, "//eye:title "):
			e.Title = strings.TrimPrefix(line, "//eye:title ")
		case strings.HasPrefix(line, "//eye:topic "):
			e.Topic = strings.TrimPrefix(line, "//eye:topic ")
		default:
			body = append(body, line)
		}
	}
	e.Code = strings.TrimLeft(strings.Join(body, "\n"), "\n")
	return e
}

// All — все снипетты в порядке имён файлов (номера в именах задают порядок).
func All() []Example {
	entries, err := snippets.ReadDir("snippets")
	if err != nil {
		return []Example{}
	}
	names := make([]string, 0, len(entries))
	for _, en := range entries {
		names = append(names, en.Name())
	}
	sort.Strings(names)
	out := make([]Example, 0, len(names))
	for _, n := range names {
		raw, err := snippets.ReadFile("snippets/" + n)
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(n, ".go.txt")
		out = append(out, parse(id, raw))
	}
	return out
}
