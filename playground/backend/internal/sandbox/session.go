// Живые сеансы странствия: снипетт компилируется как обычно, но процесс
// не умирает после запуска — он говорит сеансовым протоколом Ока
// (EYE_SESSION=1, см. internal/proto библиотеки), а сервер релеит команды
// клиента в его stdin и ответы обратно.
//
// Дисциплина жизни: сеансов не больше SessionMax; сеанс умирает по явному
// Close, по простою SessionIdle и по возрасту SessionLife (жнец ходит раз
// в ReapTick). Печать пользователя (fmt.Println до и во время странствия)
// не ломает протокол: непротокольные строки собираются отдельно и уходят
// клиенту как stdout.
package sandbox

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/vitikevich-landau/go_magic_eye/playground/backend/internal/diag"
)

// Пределы сеансов (дополняют Options; нули → умолчания в New).
const (
	defaultSessionMax  = 8
	defaultSessionIdle = 3 * time.Minute
	defaultSessionLife = 30 * time.Minute
	sessionHelloWait   = 10 * time.Second
	sessionCmdWait     = 10 * time.Second
	reapTick           = 30 * time.Second
)

// ErrNoSession — программа отработала и вышла, не начав сеанс: в снипетте
// нет eye.Explore / галереи.
var ErrNoSession = errors.New(
	"снипетт не начал странствие: для живого дерева нужен eye.Explore(&объект) или галерея с Run()")

// ErrSessionGone — сеанс уже завершился (умер процесс, простой, возраст).
var ErrSessionGone = errors.New("сеанс завершился")

// Live — один живой сеанс странствия.
type Live struct {
	ID        string
	Roots     json.RawMessage // корни из hello — как есть, passthrough
	CompileMS int64

	runner *Runner
	cmd    *exec.Cmd
	stdin  interface{ Write([]byte) (int, error) }
	lines  chan []byte // протокольные строки stdout

	mu       sync.Mutex // одна команда в полёте
	nextID   int
	lastUsed time.Time
	born     time.Time

	noiseMu sync.Mutex
	noise   strings.Builder // непротокольный stdout — печать пользователя
	stderr  *capBuffer

	dir       string
	closeOnce sync.Once
	dead      chan struct{} // закрыт, когда процесс вышел
}

// StartSession — компиляция и запуск сеанса. Ошибка компиляции приходит
// как err=nil + res.OK=false с диагностиками (как у Run); ErrNoSession —
// программа вышла, не поздоровавшись.
func (r *Runner) StartSession(ctx context.Context, code string) (*Live, RunResult, error) {
	release, err := r.acquire(ctx)
	if err != nil {
		return nil, RunResult{}, err
	}
	defer release()

	if n := r.sessionCount(); n >= r.opts.SessionMax {
		return nil, RunResult{}, fmt.Errorf("%w: живых сеансов уже %d", ErrBusy, n)
	}

	dir, cleanup, err := r.workdir(code)
	if err != nil {
		return nil, RunResult{}, err
	}

	t0 := time.Now()
	prog, stderr, err := r.compile(ctx, dir)
	compileMS := time.Since(t0).Milliseconds()
	if err != nil {
		cleanup()
		return nil, RunResult{OK: false, Diags: diag.Parse(stderr), Stderr: stderr, CompileMS: compileMS}, nil
	}

	s, err := r.launchSession(prog, dir, compileMS)
	if err != nil {
		cleanup()
		return nil, RunResult{}, err
	}
	if err := s.awaitHello(); err != nil {
		s.Close()
		return nil, RunResult{}, err
	}
	r.registerSession(s)
	return s, RunResult{OK: true, Diags: nil, CompileMS: compileMS}, nil
}

func (r *Runner) launchSession(prog, dir string, compileMS int64) (*Live, error) {
	argv := []string{prog}
	if r.opts.Isolate {
		argv = append([]string{"unshare", "-r", "-n"}, argv...)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=" + os.TempDir(),
		"EYE_SESSION=1",
		"GOMEMLIMIT=" + r.opts.MemLimit,
	}
	setProcGroup(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	s := &Live{
		ID:        newSessionID(),
		CompileMS: compileMS,
		runner:    r,
		cmd:       cmd,
		stdin:     stdin,
		lines:     make(chan []byte, 16),
		lastUsed:  time.Now(),
		born:      time.Now(),
		stderr:    newCapBuffer(r.opts.MaxOutput),
		dir:       dir,
		dead:      make(chan struct{}),
	}
	cmd.Stderr = s.stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go s.pump(stdout)
	go func() {
		cmd.Wait()
		close(s.dead)
	}()
	return s, nil
}

// pump — разбор stdout процесса: протокольные строки (JSON-объект с id или
// hello) уходят в канал, всё прочее — печать пользователя, копится в noise.
func (s *Live) pump(stdout interface{ Read([]byte) (int, error) }) {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if isProtocolLine(line) {
			cp := make([]byte, len(line))
			copy(cp, line)
			select {
			case s.lines <- cp:
			default: // клиент не ждёт ответа — строку некому отдать
			}
			continue
		}
		s.noiseMu.Lock()
		if s.noise.Len() < int(s.runner.opts.MaxOutput) {
			s.noise.Write(line)
			s.noise.WriteByte('\n')
		}
		s.noiseMu.Unlock()
	}
	close(s.lines)
}

// isProtocolLine — строка сеансового протокола: JSON-объект с полем id
// (ответ) или eye_session_version (hello).
func isProtocolLine(line []byte) bool {
	if len(line) == 0 || line[0] != '{' {
		return false
	}
	var probe struct {
		ID      *int `json:"id"`
		Version *int `json:"eye_session_version"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		return false
	}
	return probe.ID != nil || probe.Version != nil
}

// awaitHello — дождаться рукопожатия или честно объяснить, почему его нет.
func (s *Live) awaitHello() error {
	deadline := time.After(sessionHelloWait)
	for {
		select {
		case line, open := <-s.lines:
			if !open {
				return ErrNoSession
			}
			var hi struct {
				Version int             `json:"eye_session_version"`
				Roots   json.RawMessage `json:"roots"`
			}
			if json.Unmarshal(line, &hi) == nil && hi.Version >= 1 {
				s.Roots = hi.Roots
				return nil
			}
			// протокольная строка, но не hello — не наш случай, ждём дальше
		case <-s.dead:
			return ErrNoSession
		case <-deadline:
			return fmt.Errorf("странствие не началось за %s", sessionHelloWait)
		}
	}
}

// Do — команда сеансу: kids/detail. Возвращает сырой ответ протокола
// (passthrough клиенту) — сервер в содержимое не вмешивается.
func (s *Live) Do(cmd string, node int) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.dead:
		return nil, ErrSessionGone
	default:
	}
	s.lastUsed = time.Now()

	s.nextID++
	req, _ := json.Marshal(map[string]any{"id": s.nextID, "cmd": cmd, "node": node})
	req = append(req, '\n')
	if _, err := s.stdin.Write(req); err != nil {
		return nil, ErrSessionGone
	}

	deadline := time.After(sessionCmdWait)
	for {
		select {
		case line, open := <-s.lines:
			if !open {
				return nil, ErrSessionGone
			}
			var probe struct {
				ID int `json:"id"`
			}
			if json.Unmarshal(line, &probe) == nil && probe.ID == s.nextID {
				return json.RawMessage(line), nil
			}
			// чужой id (запоздалый ответ) — пропускаем
		case <-deadline:
			return nil, fmt.Errorf("сеанс молчит дольше %s", sessionCmdWait)
		}
	}
}

// Noise — накопленная печать пользователя; сбрасывается при чтении
// (клиент дочитывает поток порциями).
func (s *Live) Noise() string {
	s.noiseMu.Lock()
	defer s.noiseMu.Unlock()
	out := s.noise.String()
	s.noise.Reset()
	return out
}

// Close — вежливый quit, затем контрольное убийство группы и уборка.
func (s *Live) Close() {
	s.closeOnce.Do(func() {
		if s.stdin != nil {
			s.nextID++
			req, _ := json.Marshal(map[string]any{"id": s.nextID, "cmd": "quit"})
			s.stdin.Write(append(req, '\n'))
		}
		select {
		case <-s.dead:
		case <-time.After(time.Second):
			killProcGroup(s.cmd)
			<-s.dead
		}
		os.RemoveAll(s.dir)
		s.runner.unregisterSession(s.ID)
	})
}

// ── реестр сеансов на Runner ─────────────────────────────────────────

func (r *Runner) registerSession(s *Live) {
	r.sessMu.Lock()
	if r.sessions == nil {
		r.sessions = map[string]*Live{}
	}
	r.sessions[s.ID] = s
	r.sessMu.Unlock()
	r.reaperOnce.Do(func() { go r.reapLoop() })
}

func (r *Runner) unregisterSession(id string) {
	r.sessMu.Lock()
	delete(r.sessions, id)
	r.sessMu.Unlock()
}

// Session — живой сеанс по id (nil — нет такого).
func (r *Runner) Session(id string) *Live {
	r.sessMu.Lock()
	defer r.sessMu.Unlock()
	return r.sessions[id]
}

func (r *Runner) sessionCount() int {
	r.sessMu.Lock()
	defer r.sessMu.Unlock()
	return len(r.sessions)
}

// reapLoop — жнец: закрывает простаивающие и зажившиеся сеансы.
func (r *Runner) reapLoop() {
	tick := r.opts.ReapTick
	if tick == 0 {
		tick = reapTick
	}
	for range time.Tick(tick) {
		r.sessMu.Lock()
		var doomed []*Live
		now := time.Now()
		for _, s := range r.sessions {
			s.mu.Lock()
			idle := now.Sub(s.lastUsed)
			age := now.Sub(s.born)
			s.mu.Unlock()
			if idle > r.opts.SessionIdle || age > r.opts.SessionLife {
				doomed = append(doomed, s)
			}
		}
		r.sessMu.Unlock()
		for _, s := range doomed {
			s.Close() // сам вычеркнет себя из реестра
		}
	}
}

func newSessionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
