package eye

import (
	"os"
	"strconv"

	"github.com/vitikevich-landau/go_magic_eye/internal/render"
	"github.com/vitikevich-landau/go_magic_eye/internal/term"
	"github.com/vitikevich-landau/go_magic_eye/internal/text"
)

// Настройки Ока читаются из окружения — те же рычаги, что в C++-версии:
//
//	EYE_WIDTH=N        считать терминал N колонок (иначе определяем сами)
//	EYE_CENTER=0       не центрировать — прижать влево
//	EYE_COLOR=1/0      форсировать цвета вкл/выкл
//	EYE_FULL=1         не сворачивать длинные регионы
//	EYE_INTERACTIVE=0  Explore не входит в TUI — печатает как Inspect
//	EYE_SCRIPT="…"     исполнить клавиши (down enter q) и печатать кадры
//	EYE_HEIGHT=N       высота кадра в EYE_SCRIPT-режиме (по умолчанию 40)
//	EYE_ASCII=1        рамки и стрелки — чистый ASCII
//	EYE_SNAP_DIR=…     каталог для снимков экрана клавишей s

type config struct {
	width  int
	center bool
	full   bool
}

func envBool(name string, def bool) bool {
	switch os.Getenv(name) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func envInt(name string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n > 0 {
		return n
	}
	return def
}

// loadConfig — раз за вызов: окружение могло смениться между Inspect'ами.
func loadConfig() config {
	onTTY := term.IsTerminal(os.Stdout.Fd())
	text.Color = envBool("EYE_COLOR", onTTY)
	text.ASCII = envBool("EYE_ASCII", false)

	screen := envInt("EYE_WIDTH", 0)
	if screen == 0 {
		if w, _, ok := term.Size(); ok {
			screen = w
		} else {
			screen = 100
		}
	}
	return config{
		width:  screen,
		center: envBool("EYE_CENTER", true),
		full:   envBool("EYE_FULL", false),
	}
}

// renderOptions — ширина рамки: не шире экрана и не безумно широко.
func (c config) renderOptions() render.Options {
	w := c.width
	if w > 110 {
		w = 110
	}
	return render.Options{Width: w, Full: c.full}
}
