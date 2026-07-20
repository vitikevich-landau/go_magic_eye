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
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	MemLimit       string        // GOMEMLIMIT запущенной программы (мягкая цель GC)
	HardMemMiB     int           // RLIMIT_AS снипетта, МиБ (1024; -1 — выключить)
	Concurrency    int           // одновременных сборок (NumCPU)
	QueueWait      time.Duration // ожидание места в очереди до ErrBusy (15 с)
	Isolate        bool          // оборачивать запуск в unshare -rn (без сети)

	// пределы живых сеансов странствия (session.go)
	SessionMax  int           // одновременных сеансов (8)
	SessionIdle time.Duration // смерть по простою (3 мин)
	SessionLife time.Duration // потолок возраста (30 мин)
	HelloWait   time.Duration // ожидание рукопожатия после старта (10 с)
	ReapTick    time.Duration // шаг жнеца (30 с; тестам нужен мельче)
}

// Runner — песочница с очередью: не больше Concurrency сборок разом.
type Runner struct {
	opts Options
	sem  chan struct{}

	sessMu      sync.Mutex
	sessions    map[string]*Live
	sessPending int // брони под стартующие сеансы (reserveSessionSlot)
	reaperOnce  sync.Once
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
	if opts.HardMemMiB == 0 {
		// Go-рантайм резервирует сотни МиБ адресного пространства уже на
		// старте (замер: снипетту нужно 512–768 МиБ, и цифра плавает с
		// числом потоков) — потолок берётся с двойным запасом: 2 ГиБ всё
		// ещё настоящий заслон против безразмерного обжорства, но не
		// роняет честные снипетты на границе резервов рантайма
		opts.HardMemMiB = 2048
	}
	if opts.Concurrency == 0 {
		opts.Concurrency = runtime.NumCPU()
	}
	if opts.QueueWait == 0 {
		opts.QueueWait = 15 * time.Second
	}
	if opts.SessionMax == 0 {
		opts.SessionMax = defaultSessionMax
	}
	if opts.SessionIdle == 0 {
		opts.SessionIdle = defaultSessionIdle
	}
	if opts.SessionLife == 0 {
		opts.SessionLife = defaultSessionLife
	}
	if opts.HelloWait == 0 {
		opts.HelloWait = defaultHelloWait
	}
	return &Runner{opts: opts, sem: make(chan struct{}, opts.Concurrency)}
}

// MaxCode — лимит размера снипетта (нужен API для MaxBytesReader).
func (r *Runner) MaxCode() int64 { return r.opts.MaxCode }

// ErrBusy — очередь сборок переполнена; API отвечает 429.
var ErrBusy = fmt.Errorf("песочница занята: слишком много одновременных сборок")

// acquire — место в очереди или ErrBusy. Ожидание ограничено QueueWait
// ЗДЕСЬ, а не дедлайном входного ctx: иначе таймаут очереди дожил бы до
// компиляции и втихую урезал бы CompileTimeout (ctx остаётся родителем —
// отмена запроса клиентом сработает).
func (r *Runner) acquire(ctx context.Context) (release func(), err error) {
	qctx, cancel := context.WithTimeout(ctx, r.opts.QueueWait)
	defer cancel()
	select {
	case r.sem <- struct{}{}:
		return func() { <-r.sem }, nil
	case <-qctx.Done():
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
	ExitCode  int         `json:"exit_code"` // код выхода (0 — чисто; смысла нет при timed_out)
	CompileMS int64       `json:"compile_ms"`
	RunMS     int64       `json:"run_ms"`
}

// notMainDiag — снипетт не package main: честная диагностика с позицией
// вместо попытки запустить архив (go build -o для не-main пакета успешно
// пишет объект, не бинарь — компилятор тут не заругается, а запуск упал бы
// сбоем песочницы). Синтаксический мусор не наш случай: его в полный рост
// обругает настоящий компилятор.
func notMainDiag(code string) *diag.Diag {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", code, parser.PackageClauseOnly)
	if err != nil || f.Name == nil || f.Name.Name == "main" {
		return nil
	}
	pos := fset.Position(f.Name.Pos())
	return &diag.Diag{
		Line: pos.Line, Col: pos.Column, Severity: "error",
		Message: fmt.Sprintf("снипетт должен быть package main — package %s не собирается в программу", f.Name.Name),
	}
}

// Check — только компиляция: диагностики для маркеров редактора.
func (r *Runner) Check(ctx context.Context, code string) (CheckResult, error) {
	if d := notMainDiag(code); d != nil {
		return CheckResult{OK: false, Diags: []diag.Diag{*d}}, nil
	}
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
		return CheckResult{OK: false, Diags: diagsOrFallback(stderr)}, nil
	}
	return CheckResult{OK: true, Diags: []diag.Diag{}}, nil
}

// Run — компиляция и запуск. Ошибка компиляции — не error: она законный
// результат с диагностиками. error — только сбой самой песочницы.
func (r *Runner) Run(ctx context.Context, code string) (RunResult, error) {
	if d := notMainDiag(code); d != nil {
		return RunResult{OK: false, Diags: []diag.Diag{*d}}, nil
	}
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
			OK: false, Diags: diagsOrFallback(stderr),
			Stderr: stderr, CompileMS: compileMS,
		}, nil
	}

	t1 := time.Now()
	res, err := r.execute(prog)
	if err != nil {
		// песочница не смогла ЗАПУСТИТЬ программу (сломанная обёртка,
		// отказ prlimit) — это сбой сервиса, не результат снипетта
		return RunResult{}, fmt.Errorf("запуск в песочнице: %w", err)
	}
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
	// путь в replace всегда в кавычках (%q): пробел в каталоге (EYE_PG_LIB,
	// «Magic Eye/…») иначе рассыпал бы директиву на токены и валил парсинг
	// go.mod ещё до компиляции
	gomod := fmt.Sprintf(
		"module snippet\n\ngo 1.22\n\nrequire github.com/vitikevich-landau/go_magic_eye v0.0.0\n\nreplace github.com/vitikevich-landau/go_magic_eye => %q\n",
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
	cmd := exec.Command(r.opts.GoBin, "build", "-gcflags=-e", "-o", prog, ".")
	cmd.Dir = dir
	// GOOS/GOARCH пришпилены к хосту: снипетт собирается, чтобы БЕЖАТЬ
	// здесь же, и кросс-переменные из окружения сервера дали бы бинарь
	// чужой платформы (exec format error); при дублях в Env побеждает
	// последняя запись
	cmd.Env = append(os.Environ(),
		"GOPROXY=off", "GOSUMDB=off", "GOFLAGS=-mod=mod", "CGO_ENABLED=0",
		"GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	// go build — лишь драйвер: настоящую работу делают его дети (compile,
	// link). CommandContext убил бы только драйвера, и на таймауте дети
	// доедали бы CPU уже без присмотра — поэтому своя группа процессов и
	// убийство всей группы, как у запуска снипетта
	setProcGroup(cmd)
	if err := cmd.Start(); err != nil {
		return "", "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case werr := <-done:
		if werr != nil {
			return "", errBuf.String(), werr
		}
	case <-cctx.Done():
		killProcGroup(cmd)
		<-done
		// причина уходит и в stderr: у таймаута нет вывода компилятора,
		// а пустой отказ (ok:false без единого слова) хуже честного
		msg := fmt.Sprintf("компиляция не уложилась в %s", r.opts.CompileTimeout)
		if ctx.Err() != nil {
			msg = "сборка прервана: запрос отменён клиентом"
		}
		return "", msg, fmt.Errorf("%s", msg)
	}
	return prog, "", nil
}

// diagsOrFallback — позиционные диагностики из stderr компилятора; если
// парсер не выудил ни одной (таймаут, «go:»-ошибки модуля), беда приходит
// одной диагностикой 1:1 с сырым текстом — маркер в редакторе будет всегда.
func diagsOrFallback(stderr string) []diag.Diag {
	ds := diag.Parse(stderr)
	if len(ds) == 0 {
		if msg := strings.TrimSpace(stderr); msg != "" {
			if len(msg) > 500 {
				msg = msg[:500] + "…"
			}
			ds = append(ds, diag.Diag{Line: 1, Col: 1, Severity: "error", Message: msg})
		}
	}
	return ds
}

// execute — запуск собранной программы с минимальным окружением, лимитами
// памяти (мягкий GOMEMLIMIT + жёсткий RLIMIT_AS), обрезанием вывода и
// убийством всей группы процессов по таймауту. error — программа НЕ
// ЗАПУСТИЛАСЬ (это сбой песочницы, не результат снипетта).
func (r *Runner) execute(prog string) (RunResult, error) {
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
		return RunResult{}, err
	}
	if err := applyMemLimit(cmd.Process.Pid, int64(r.opts.HardMemMiB)<<20); err != nil {
		killProcGroup(cmd)
		cmd.Wait()
		return RunResult{}, fmt.Errorf("prlimit: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			// ненулевой выход (паника, out of memory об RLIMIT_AS) — законный
			// учебный результат: код выхода уйдёт в ExitCode, беда — в stderr
			_ = err
		}
	case <-time.After(r.opts.RunTimeout):
		res.TimedOut = true
		res.Stderr = fmt.Sprintf("⏱ программа убита: не уложилась в %s (бесконечный цикл?)", r.opts.RunTimeout)
		killProcGroup(cmd)
		<-done
	}
	// группа добивается и при НОРМАЛЬНОМ выходе: снипетт мог оставить
	// фоновых детей (exec.Command(…).Start()), которые иначе пережили бы
	// уборку каталога и копились от запуска к запуску
	killProcGroup(cmd)

	if st := cmd.ProcessState; st != nil {
		res.ExitCode = st.ExitCode()
	}
	env, rest := ExtractEnvelopes(stdout.Bytes())
	res.Envelope = env
	res.Stdout = rest
	if !res.TimedOut {
		res.Stderr = stderrBuf.String()
	}
	return res, nil
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
