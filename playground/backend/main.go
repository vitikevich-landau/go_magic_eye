// Playground Ока: веб-сервер. Раздаёт фронтенд (вшит go:embed) и API:
// компиляция снипеттов с диагностиками, запуск в песочнице, галерея примеров.
//
// Настройка — переменные EYE_PG_* (все со внятными умолчаниями):
//
//	EYE_PG_ADDR=:8080        адрес сервера
//	EYE_PG_LIB=…             корень библиотеки Ока (иначе ищется вверх от cwd)
//	EYE_PG_RUN_TIMEOUT=5s    потолок работы снипетта
//	EYE_PG_COMPILE_TIMEOUT=30s
//	EYE_PG_ISOLATE=auto      auto|on|off — запуск в unshare -r -n (без сети)
package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vitikevich-landau/go_magic_eye/playground/backend/internal/api"
	"github.com/vitikevich-landau/go_magic_eye/playground/backend/internal/sandbox"
)

//go:embed all:web
var webFS embed.FS

func main() {
	log.SetFlags(log.Ltime)

	libDir, err := findLib()
	if err != nil {
		log.Fatalf("библиотека Ока не найдена: %v (подскажи каталог через EYE_PG_LIB)", err)
	}
	isolate := decideIsolation(os.Getenv("EYE_PG_ISOLATE"))
	runner := sandbox.New(sandbox.Options{
		LibDir:         libDir,
		CompileTimeout: envDuration("EYE_PG_COMPILE_TIMEOUT", 30*time.Second),
		RunTimeout:     envDuration("EYE_PG_RUN_TIMEOUT", 5*time.Second),
		Isolate:        isolate,
	})

	// собранный фронт живёт в web/dist (кладёт vite, вшивает go:embed);
	// нет его — раздаётся заглушка web/index.html с инструкцией сборки
	webDir := "web"
	if _, err := fs.Stat(webFS, "web/dist/index.html"); err == nil {
		webDir = "web/dist"
	}
	web, err := fs.Sub(webFS, webDir)
	if err != nil {
		log.Fatal(err)
	}
	handler := api.New(runner, http.FileServerFS(web))

	addr := os.Getenv("EYE_PG_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	go func() {
		log.Printf("👁 playground Ока: http://localhost%s (библиотека: %s, изоляция сети: %v)",
			addr, libDir, isolate)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Print("останавливаюсь…")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// findLib — корень библиотеки Ока: EYE_PG_LIB или поиск go.mod с её module
// вверх от текущего каталога (в дев-режиме бэкенд запускают из playground/
// или playground/backend — библиотека лежит выше).
func findLib() (string, error) {
	if dir := os.Getenv("EYE_PG_LIB"); dir != "" {
		return filepath.Abs(dir)
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for range 6 {
		mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(mod), "module github.com/vitikevich-landau/go_magic_eye\n") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("go.mod библиотеки не найден вверх от %s", mustGetwd())
}

func mustGetwd() string {
	d, _ := os.Getwd()
	return d
}

// decideIsolation — on/off буквально; auto (и пусто) — пробуем unshare один
// раз: в контейнерах user namespaces часто запрещены, тогда честно едем без
// сетевой изоляции (GOPROXY=off всё равно не даст ничего скачать на сборке).
func decideIsolation(mode string) bool {
	switch strings.ToLower(mode) {
	case "on", "1", "true":
		return true
	case "off", "0", "false":
		return false
	}
	ok := sandbox.ProbeIsolation()
	if !ok {
		log.Print("unshare недоступен — снипетты бегут без сетевой изоляции")
	}
	return ok
}

func envDuration(name string, def time.Duration) time.Duration {
	if v := os.Getenv(name); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}
