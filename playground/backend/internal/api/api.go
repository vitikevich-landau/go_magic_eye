// Package api — HTTP-ручки playground: check (диагностики для маркеров
// редактора), run (полный прогон с JSON-конвертом Ока), examples (галерея
// снипеттов). Все ответы — JSON; контракт — playground/SPEC.md §4.1.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/vitikevich-landau/go_magic_eye/playground/backend/internal/diag"
	"github.com/vitikevich-landau/go_magic_eye/playground/backend/internal/examples"
	"github.com/vitikevich-landau/go_magic_eye/playground/backend/internal/sandbox"
)

// Server — API поверх песочницы.
type Server struct {
	runner *sandbox.Runner
}

// New — маршрутизатор API. static — файловая система фронтенда (go:embed);
// nil — раздаётся только API (удобно в тестах).
func New(runner *sandbox.Runner, static http.Handler) http.Handler {
	s := &Server{runner: runner}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/check", s.handleCheck)
	mux.HandleFunc("POST /api/run", s.handleRun)
	mux.HandleFunc("POST /api/explore", s.handleExplore)
	mux.HandleFunc("POST /api/explore/cmd", s.handleExploreCmd)
	mux.HandleFunc("POST /api/explore/close", s.handleExploreClose)
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
	// дедлайн тут не навешивается: очередь ограничивает себя сама
	// (Options.QueueWait), а компиляции положен её полный CompileTimeout
	res, err := s.runner.Check(r.Context(), code)
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
	res, err := s.runner.Run(r.Context(), code)
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

// ── странствие: живые сеансы ─────────────────────────────────────────

// exploreResponse — ответ POST /api/explore.
type exploreResponse struct {
	OK          bool            `json:"ok"`
	Diagnostics []diag.Diag     `json:"diagnostics"`
	Session     string          `json:"session,omitempty"`
	Roots       json.RawMessage `json:"roots,omitempty"`
	Stdout      string          `json:"stdout,omitempty"`
	Stderr      string          `json:"stderr,omitempty"`
	Error       string          `json:"error,omitempty"`
	CompileMS   int64           `json:"compile_ms"`
}

func (s *Server) handleExplore(w http.ResponseWriter, r *http.Request) {
	code, ok := s.readCode(w, r)
	if !ok {
		return
	}
	live, res, err := s.runner.StartSession(r.Context(), code)
	switch {
	case errors.Is(err, sandbox.ErrNoSession):
		writeJSON(w, http.StatusOK, exploreResponse{
			OK: false, Diagnostics: []diag.Diag{},
			Error: err.Error(),
		})
		return
	case err != nil:
		s.sandboxError(w, err)
		return
	case !res.OK: // ошибка компиляции — диагностики для маркеров
		writeJSON(w, http.StatusOK, exploreResponse{
			OK: false, Diagnostics: res.Diags, Stderr: res.Stderr, CompileMS: res.CompileMS,
		})
		return
	}
	writeJSON(w, http.StatusOK, exploreResponse{
		OK: true, Diagnostics: []diag.Diag{},
		Session: live.ID, Roots: live.Roots,
		Stdout: live.Noise(), CompileMS: live.CompileMS,
	})
}

type exploreCmdRequest struct {
	Session string `json:"session"`
	Cmd     string `json:"cmd"`
	Node    int    `json:"node"`
}

func (s *Server) handleExploreCmd(w http.ResponseWriter, r *http.Request) {
	var req exploreCmdRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "ожидался JSON {session, cmd, node}")
		return
	}
	// наружу торчат только команды чтения: quit, пущенный сюда, завершил бы
	// процесс, оставив мёртвый сеанс занимать слот SessionMax до жнеца —
	// закрытие идёт своей ручкой (/api/explore/close), она и вычёркивает
	if req.Cmd != "kids" && req.Cmd != "detail" {
		writeError(w, http.StatusBadRequest, "команда не из странствия: только kids и detail (закрытие — /api/explore/close)")
		return
	}
	live := s.runner.Session(req.Session)
	if live == nil {
		writeError(w, http.StatusNotFound, "сеанса нет: истёк или не существовал")
		return
	}
	raw, err := live.Do(req.Cmd, req.Node)
	if err != nil {
		if errors.Is(err, sandbox.ErrSessionGone) {
			live.Close()
			writeError(w, http.StatusGone, "сеанс завершился (программа вышла или убита)")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// ответ протокола — passthrough; свежая печать пользователя — довеском
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		writeError(w, http.StatusInternalServerError, "сеанс ответил не-JSON'ом")
		return
	}
	if noise := live.Noise(); noise != "" {
		resp["stdout"] = noise
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleExploreClose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Session string `json:"session"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "ожидался JSON {session}")
		return
	}
	if live := s.runner.Session(req.Session); live != nil {
		live.Close()
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// readCode — тело запроса с лимитом размера снипетта; false — ответ уже ушёл.
// Лимит меряется по ДЕКОДИРОВАННОМУ коду: JSON-обёртка и экранирование
// (\n, кавычки, \uXXXX) раздувают тело, и потолок на всё тело отвергал бы
// снипетты, которые песочница честно принимает. Тело ограничено кратно —
// только как страховка от мусорных мегабайтов.
func (s *Server) readCode(w http.ResponseWriter, r *http.Request) (string, bool) {
	body := http.MaxBytesReader(w, r.Body, 4*s.runner.MaxCode()+4096)
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
	if int64(len(req.Code)) > s.runner.MaxCode() {
		writeError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("снипетт больше %d КиБ", s.runner.MaxCode()>>10))
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
