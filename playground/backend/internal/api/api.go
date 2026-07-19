// Package api — HTTP-ручки playground: check (диагностики для маркеров
// редактора), run (полный прогон с JSON-конвертом Ока), examples (галерея
// снипеттов). Все ответы — JSON; контракт — playground/SPEC.md §4.1.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/vitikevich-landau/go_magic_eye/playground/backend/internal/examples"
	"github.com/vitikevich-landau/go_magic_eye/playground/backend/internal/sandbox"
)

// Server — API поверх песочницы.
type Server struct {
	runner *sandbox.Runner
	// queueWait — сколько запрос готов стоять в очереди за слотом песочницы,
	// прежде чем получить 429
	queueWait time.Duration
}

// New — маршрутизатор API. static — файловая система фронтенда (go:embed);
// nil — раздаётся только API (удобно в тестах).
func New(runner *sandbox.Runner, static http.Handler) http.Handler {
	s := &Server{runner: runner, queueWait: 15 * time.Second}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/check", s.handleCheck)
	mux.HandleFunc("POST /api/run", s.handleRun)
	mux.HandleFunc("GET /api/examples", s.handleExamples)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	if static != nil {
		mux.Handle("GET /", static)
	}
	return logRequests(mux)
}

type codeRequest struct {
	Code string `json:"code"`
}

// runResponse — ответ /api/run: результат песочницы + конверт Ока как сырой
// JSON (его собрала библиотека, пересобирать бессмысленно).
type runResponse struct {
	sandbox.RunResult
	Eye json.RawMessage `json:"eye"`
}

func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	code, ok := s.readCode(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.queueWait)
	defer cancel()
	res, err := s.runner.Check(ctx, code)
	if err != nil {
		s.sandboxError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	code, ok := s.readCode(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.queueWait)
	defer cancel()
	res, err := s.runner.Run(ctx, code)
	if err != nil {
		s.sandboxError(w, err)
		return
	}
	resp := runResponse{RunResult: res}
	if res.Envelope != nil {
		resp.Eye = json.RawMessage(res.Envelope)
	} else {
		resp.Eye = json.RawMessage("null")
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleExamples(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, examples.All())
}

// readCode — тело запроса с лимитом размера снипетта; false — ответ уже ушёл.
func (s *Server) readCode(w http.ResponseWriter, r *http.Request) (string, bool) {
	body := http.MaxBytesReader(w, r.Body, s.runner.MaxCode())
	var req codeRequest
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "снипетт слишком большой")
			return "", false
		}
		writeError(w, http.StatusBadRequest, "ожидался JSON {\"code\": \"…\"}")
		return "", false
	}
	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "пустой снипетт")
		return "", false
	}
	return req.Code, true
}

func (s *Server) sandboxError(w http.ResponseWriter, err error) {
	if errors.Is(err, sandbox.ErrBusy) {
		writeError(w, http.StatusTooManyRequests, err.Error())
		return
	}
	log.Printf("песочница: %v", err)
	writeError(w, http.StatusInternalServerError, "сбой песочницы: "+err.Error())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		log.Printf("ответ не ушёл: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// logRequests — одна строка на запрос: метод, путь, статус, длительность.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t0 := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s → %d (%s)", r.Method, r.URL.Path, sw.status, time.Since(t0).Round(time.Millisecond))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// ReadFrom — пробрасываем io.Copy-оптимизацию файлового сервера.
func (w *statusWriter) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(w.ResponseWriter, r)
}
