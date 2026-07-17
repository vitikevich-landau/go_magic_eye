package tui

import (
	"fmt"
	"io"
)

// Script-режим (EYE_SCRIPT="down enter q"): исполнить клавиши из строки и
// напечатать кадры в stdout. Терминал не нужен — так странствие живёт в CI
// и покрывается снапшот-тестами.

var scriptNames = map[string]Key{
	"up": {Type: KUp}, "down": {Type: KDown},
	"left": {Type: KLeft}, "right": {Type: KRight},
	"enter": {Type: KEnter}, "esc": {Type: KEsc},
	"tab": {Type: KTab}, "backspace": {Type: KBackspace},
	"pgup": {Type: KPgUp}, "pgdn": {Type: KPgDn},
	"home": {Type: KHome}, "end": {Type: KEnd}, "f1": {Type: KF1},
}

// ParseScriptKey — токен скрипта → клавиша (имя или одиночная руна).
func ParseScriptKey(tok string) (Key, bool) {
	if k, ok := scriptNames[tok]; ok {
		return k, true
	}
	r := []rune(tok)
	if len(r) == 1 {
		return Key{Type: KRune, R: r[0]}, true
	}
	return Key{}, false
}

// RunScript — кадр за кадром: начальный, затем после каждой клавиши.
func (a *App) RunScript(tokens []string, w io.Writer, width, height int) {
	a.W, a.H = width, height
	frame := func(title string) {
		fmt.Fprintf(w, "══ кадр: %s ══\n", title)
		for _, l := range a.Frame() {
			fmt.Fprintln(w, l)
		}
	}
	frame("старт")
	for _, tok := range tokens {
		k, ok := ParseScriptKey(tok)
		if !ok {
			fmt.Fprintf(w, "── клавиша «%s» не понята — пропущена ──\n", tok)
			continue
		}
		if a.Handle(k) {
			fmt.Fprintf(w, "══ выход по «%s» ══\n", tok)
			return
		}
		frame(tok)
	}
}
