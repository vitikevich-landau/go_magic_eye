// Package sandbox — компиляция и запуск пользовательского кода.
//
// Модель угроз мягкая (учебный инструмент, контейнер поднимают для себя),
// но от СЛУЧАЙНОГО вреда защищаемся всерьёз: таймауты на сборку и запуск,
// лимит памяти, обрезание вывода, запрет сети (unshare, где доступен),
// GOPROXY=off — посторонние модули не соберутся физически.
package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/vitikevich-landau/go_magic_eye/playground/backend/internal/diag"
)

// Options — рычаги песочницы; нули заменяются значениями по умолчанию в New.
type Options struct {
	LibDir         string        // корень библиотеки Ока (replace в go.mod снипетта)
	GoBin          string        // компилятор («go»)
	CompileTimeout time.Duration // 30 с
	RunTimeout     time.Duration // 5 с
	MaxOutput      int64         // 4 МиБ на каждый из stdout/stderr
	MaxCode        int64         // 128 КиБ на снипетт
	MemLimit       string        // GOMEMLIMIT запущенной программы
	Concurrency    int           // одновременных сборок (NumCPU)
	Isolate        bool          // оборачивать запуск в unshare -rn (без сети)
}

// Runner — песочница с очередью: не больше Concurrency сборок разом.
type Runner struct {
	opts Options
	sem  chan struct{}
}

// New — песочница с заполненными умолчаниями.
func New(opts Options) *Runner {
	if opts.GoBin == "" {
		opts.GoBin = "go"
	}
	if opts.CompileTimeout == 0 {
		opts.CompileTimeout = 30 * time.Second
	}
	if opts.RunTimeout == 0 {
		opts.RunTimeout = 5 * time.Second
	}
	if opts.MaxOutput == 0 {
		opts.MaxOutput = 4 << 20
	}
	if opts.MaxCode == 0 {
		opts.MaxCode = 128 << 10
	}
	if opts.MemLimit == "" {
		opts.MemLimit = "256MiB"
	}
	if opts.Concurrency == 0 {
		opts.Concurrency = runtime.NumCPU()
	}
	return &Runner{opts: opts, sem: make(chan struct{}, opts.Concurrency)}
}

// MaxCode — лимит размера снипетта (нужен API для MaxBytesReader).
func (r *Runner) MaxCode() int64 { return r.opts.MaxCode }

// ErrBusy — очередь сборок переполнена; API отвечает 429.
var ErrBusy = fmt.Errorf("песочница занята: слишком много одновременных сборок")

// acquire — место в очереди или ErrBusy по истечении ctx.
func (r *Runner) acquire(ctx context.Context) (release func(), err error) {
	select {
	case r.sem <- struct{}{}:
		return func() { <-r.sem }, nil
	case <-ctx.Done():
		return nil, ErrBusy
	}
}

// CheckResult — итог компиляции без запуска.
type CheckResult struct {
	OK    bool        `json:"ok"`
	Diags []diag.Diag `json:"diagnostics"`
}

// RunResult — итог полного прогона: компиляция + запуск + разбор вывода.
type RunResult struct {
	OK        bool        `json:"ok"`
	Diags     []diag.Diag `json:"diagnostics"`
	Envelope  []byte      `json:"-"`      // слитый JSON-конверт Ока (nil — Око молчало)
	Stdout    string      `json:"stdout"` // вывод программы БЕЗ конвертов
	Stderr    string      `json:"stderr"`
	TimedOut  bool        `json:"timed_out"`
	CompileMS int64       `json:"compile_ms"`
	RunMS     int64       `json:"run_ms"`
}

// Check — только компиляция: диагностики для маркеров редактора.
func (r *Runner) Check(ctx context.Context, code string) (CheckResult, error) {
	release, err := r.acquire(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	defer release()

	dir, cleanup, err := r.workdir(code)
	if err != nil {
		return CheckResult{}, err
	}
	defer cleanup()

	_, stderr, err := r.compile(ctx, dir)
	if err != nil {
		return CheckResult{OK: false, Diags: diag.Parse(stderr)}, nil
	}
	return CheckResult{OK: true, Diags: []diag.Diag{}}, nil
}

// Run — компиляция и запуск. Ошибка компиляции — не error: она законный
// результат с диагностиками. error — только сбой самой песочницы.
func (r *Runner) Run(ctx context.Context, code string) (RunResult, error) {
	release, err := r.acquire(ctx)
	if err != nil {
		return RunResult{}, err
	}
	defer release()

	dir, cleanup, err := r.workdir(code)
	if err != nil {
		return RunResult{}, err
	}
	defer cleanup()

	t0 := time.Now()
	prog, stderr, err := r.compile(ctx, dir)
	compileMS := time.Since(t0).Milliseconds()
	if err != nil {
		return RunResult{
			OK: false, Diags: diag.Parse(stderr),
			Stderr: stderr, CompileMS: compileMS,
		}, nil
	}

	t1 := time.Now()
	res := r.execute(prog)
	res.OK = !res.TimedOut
	res.Diags = []diag.Diag{}
	res.CompileMS = compileMS
	res.RunMS = time.Since(t1).Milliseconds()
	return res, nil
}

// workdir — временный каталог со снипеттом: main.go как есть (без обёрток —
// позиции диагностик честные) + go.mod с replace на локальную библиотеку.
func (r *Runner) workdir(code string) (dir string, cleanup func(), err error) {
	if int64(len(code)) > r.opts.MaxCode {
		return "", nil, fmt.Errorf("снипетт больше %d КиБ", r.opts.MaxCode>>10)
	}
	dir, err = os.MkdirTemp("", "eye-run-*")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.RemoveAll(dir) }
	gomod := fmt.Sprintf(
		"module snippet\n\ngo 1.22\n\nrequire github.com/vitikevich-landau/go_magic_eye v0.0.0\n\nreplace github.com/vitikevich-landau/go_magic_eye => %s\n",
		r.opts.LibDir)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(code), 0o644); err == nil {
		err = os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644)
	}
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return dir, cleanup, nil
}

// compile — `go build -gcflags=-e`: все ошибки, не первые десять.
// GOPROXY=off отрезает сеть на этапе сборки: кроме stdlib и Ока (replace на
// локальный каталог) собрать нечего — фильтр импортов не нужен.
func (r *Runner) compile(ctx context.Context, dir string) (prog, stderr string, err error) {
	cctx, cancel := context.WithTimeout(ctx, r.opts.CompileTimeout)
	defer cancel()
	prog = filepath.Join(dir, "prog")
	cmd := exec.CommandContext(cctx, r.opts.GoBin, "build", "-gcflags=-e", "-o", prog, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GOPROXY=off", "GOSUMDB=off", "GOFLAGS=-mod=mod", "CGO_ENABLED=0")
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if cctx.Err() != nil {
			return "", "", fmt.Errorf("компиляция не уложилась в %s", r.opts.CompileTimeout)
		}
		return "", errBuf.String(), err
	}
	return prog, "", nil
}

// execute — запуск собранной программы с минимальным окружением, лимитом
// памяти, обрезанием вывода и убийством всей группы процессов по таймауту.
func (r *Runner) execute(prog string) RunResult {
	argv := []string{prog}
	if r.opts.Isolate {
		// unshare -r: uid 0 внутри новой user-ns (иначе -n требует root);
		// -n: своя пустая сетевая ns — сети у снипетта нет
		argv = append([]string{"unshare", "-r", "-n"}, argv...)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = filepath.Dir(prog)
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=" + os.TempDir(),
		"EYE_FORMAT=json",
		"EYE_INTERACTIVE=0",
		"GOMEMLIMIT=" + r.opts.MemLimit,
	}
	stdout := newCapBuffer(r.opts.MaxOutput)
	stderrBuf := newCapBuffer(r.opts.MaxOutput)
	cmd.Stdout = stdout
	cmd.Stderr = stderrBuf
	setProcGroup(cmd)

	res := RunResult{}
	if err := cmd.Start(); err != nil {
		res.Stderr = "песочница: " + err.Error()
		return res
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			// ненулевой выход (паника и т.п.) — законный учебный результат,
			// он уже виден в stderr; отдельно не помечаем
			_ = err
		}
	case <-time.After(r.opts.RunTimeout):
		killProcGroup(cmd)
		<-done
		res.TimedOut = true
		res.Stderr = fmt.Sprintf("⏱ программа убита: не уложилась в %s (бесконечный цикл?)", r.opts.RunTimeout)
	}

	env, rest := ExtractEnvelopes(stdout.Bytes())
	res.Envelope = env
	res.Stdout = rest
	if !res.TimedOut {
		res.Stderr = stderrBuf.String()
	}
	return res
}

// capBuffer — писатель с потолком: хвост сверх лимита молча отбрасывается,
// факт обрезания помечается в конце.
type capBuffer struct {
	buf     bytes.Buffer
	left    int64
	clipped bool
}

func newCapBuffer(limit int64) *capBuffer { return &capBuffer{left: limit} }

func (b *capBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if int64(n) > b.left {
		p = p[:b.left]
		b.clipped = true
	}
	b.buf.Write(p)
	b.left -= int64(len(p))
	return n, nil // программе врём, что записали всё: пусть живёт до таймаута
}

func (b *capBuffer) Bytes() []byte {
	if b.clipped {
		return append(b.buf.Bytes(), []byte("\n⋯ вывод обрезан песочницей ⋯\n")...)
	}
	return b.buf.Bytes()
}

func (b *capBuffer) String() string { return string(b.Bytes()) }

// ProbeIsolation — работает ли unshare -r -n на этой машине (в контейнерах
// user-ns часто запрещены). Зовётся один раз на старте сервера.
func ProbeIsolation() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "unshare", "-r", "-n", "true").Run() == nil
}
