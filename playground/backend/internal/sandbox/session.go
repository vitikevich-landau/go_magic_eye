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
	"bytes"
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
	defaultHelloWait   = 10 * time.Second
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
	if d := notMainDiag(code); d != nil {
		return nil, RunResult{OK: false, Diags: []diag.Diag{*d}}, nil
	}
	release, err := r.acquire(ctx)
	if err != nil {
		return nil, RunResult{}, err
	}
	defer release()

	// слот сеанса бронируется АТОМАРНО до дорогой компиляции: проверка
	// счётчика и регистрация порознь дали бы пачке одновременных explore
	// пробить SessionMax (TOCTOU)
	if err := r.reserveSessionSlot(); err != nil {
		return nil, RunResult{}, err
	}
	defer r.releaseSessionSlot()

	dir, cleanup, err := r.workdir(code)
	if err != nil {
		return nil, RunResult{}, err
	}

	t0 := time.Now()
	prog, stderr, err := r.compile(ctx, dir)
	compileMS := time.Since(t0).Milliseconds()
	if err != nil {
		cleanup()
		return nil, RunResult{OK: false, Diags: diagsOrFallback(stderr), Stderr: stderr, CompileMS: compileMS}, nil
	}

	s, err := r.launchSession(prog, dir, compileMS)
	if err != nil {
		cleanup()
		return nil, RunResult{}, err
	}
	if err := s.awaitHello(ctx); err != nil {
		s.Close()
		// паника/OOM до Explore уже пойманы в stderr, а печать программы —
		// в noise насоса: отдать ОБА с отказом, голое «нет сеанса» не
		// объясняет, что успело случиться до несостоявшегося рукопожатия
		return nil, RunResult{
			Stdout: s.Noise(), Stderr: s.stderr.String(), CompileMS: compileMS,
		}, err
	}
	// клиент мог отменить запрос ровно между hello и регистрацией:
	// такой сеанс никто не получит и не закроет — не регистрируем
	if ctx.Err() != nil {
		s.Close()
		return nil, RunResult{}, fmt.Errorf("клиент ушёл, не дождавшись странствия: %w", ctx.Err())
	}
	r.registerSession(s)
	return s, RunResult{OK: true, Diags: nil, CompileMS: compileMS}, nil
}

func (r *Runner) launchSession(prog, dir string, compileMS int64) (*Live, error) {
	argv := []string{prog}
	if r.opts.Isolate {
		argv = append([]string{"unshare", "-r", "-n"}, argv...)
	}
	argv = append(r.memLauncher(), argv...) // prlimit ДО exec снипетта
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
	// пост-старт лимит — деградация без prlimit-обёртки (см. execute)
	if len(r.memLauncher()) == 0 {
		if err := applyMemLimit(cmd.Process.Pid, int64(r.opts.HardMemMiB)<<20); err != nil {
			killProcGroup(cmd)
			cmd.Wait()
			return nil, fmt.Errorf("prlimit: %w", err)
		}
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
// Склейки «префикс{протокол}» (горутина напечатала без \n ровно перед
// ответом) расклеиваются: префикс — в noise, JSON — в канал.
//
// Строка ЛЮБОЙ длины не убивает насос: bufio.Scanner на гигантской строке
// вернул бы ErrTooLong и сеанс выглядел бы мёртвым — вместо этого хвост
// сверх MaxOutput молча отбрасывается (это заведомо печать пользователя:
// протокольные кадры такими не бывают), и разбор продолжается.
func (s *Live) pump(stdout interface{ Read([]byte) (int, error) }) {
	maxLine := int(s.runner.opts.MaxOutput)
	br := bufio.NewReaderSize(stdout, 64*1024)
	line := make([]byte, 0, 4096)
	clipped := false

	flush := func() {
		if clipped {
			// протокольным кадр такого размера быть не может — только шум
			s.addNoise(append(line, []byte(" ⋯ строка обрезана песочницей ⋯")...))
		} else {
			noise, protocol := splitProtocol(line)
			if len(noise) > 0 {
				s.addNoise(noise)
			}
			if protocol != nil {
				cp := make([]byte, len(protocol))
				copy(cp, protocol)
				select {
				case s.lines <- cp:
				default: // клиент не ждёт ответа — строку некому отдать
				}
			}
		}
		line = line[:0]
		clipped = false
	}

	for {
		chunk, err := br.ReadSlice('\n')
		if n := len(chunk); n > 0 {
			c := chunk
			gotNL := c[n-1] == '\n'
			if gotNL {
				c = c[:n-1]
			}
			if room := maxLine - len(line); len(c) > room {
				c = c[:max(room, 0)]
				clipped = true
			}
			line = append(line, c...)
			if gotNL {
				flush()
			}
		}
		if err == bufio.ErrBufferFull {
			continue // строка длиннее буфера — дочитываем её кусками
		}
		if err != nil {
			if len(line) > 0 {
				flush() // хвост без \n перед закрытием пайпа
			}
			break
		}
	}
	close(s.lines)
}

// addNoise — печать пользователя в буфер (с потолком MaxOutput).
func (s *Live) addNoise(b []byte) {
	s.noiseMu.Lock()
	if s.noise.Len() < int(s.runner.opts.MaxOutput) {
		s.noise.Write(b)
		s.noise.WriteByte('\n')
	}
	s.noiseMu.Unlock()
}

// splitProtocol — делит строку stdout на печать пользователя и протокольное
// сообщение. Чистая протокольная строка → (nil, line); чистый шум →
// (line, nil); склейка «префикс{json}» → (префикс, json).
func splitProtocol(line []byte) (noise, protocol []byte) {
	if len(line) == 0 {
		return nil, nil // пустые строки — артефакты разрывов протокола
	}
	if isProtocolLine(line) {
		return nil, line
	}
	for _, marker := range [][]byte{[]byte(`{"eye_session_version"`), []byte(`{"id"`)} {
		if i := bytes.Index(line, marker); i > 0 && isProtocolLine(line[i:]) {
			return line[:i], line[i:]
		}
	}
	return line, nil
}

// isProtocolLine — строка сеансового протокола: hello ПОЛНОЙ формы
// (eye_session_version + массив roots — ровно то, что примет awaitHello)
// или ответ формы ответа — id вместе с ok. Половинных форм протокол не
// знает: лог {"id":1} или {"eye_session_version":1} без корней — печать
// пользователя, она обязана дойти до stdout, а не пропасть между двумя
// решетами (насос увёл из noise, рукопожатие отвергло). Лог, нарочно
// совпавший с полной формой, различить уже нечем — честная граница
// эвристики.
func isProtocolLine(line []byte) bool {
	if len(line) == 0 || line[0] != '{' {
		return false
	}
	var probe struct {
		ID      *int            `json:"id"`
		OK      *bool           `json:"ok"`
		Version *int            `json:"eye_session_version"`
		Roots   json.RawMessage `json:"roots"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		return false
	}
	if probe.Version != nil {
		return bytes.HasPrefix(bytes.TrimSpace(probe.Roots), []byte("["))
	}
	return probe.ID != nil && probe.OK != nil
}

// awaitHello — дождаться рукопожатия или честно объяснить, почему его нет.
// Оба исхода — «программа вышла без Explore» и «не поздоровалась за
// HelloWait» — вина снипетта, не песочницы: оба приходят классом
// ErrNoSession, чтобы API отвечал пользовательской ошибкой, а не 500.
// Отмена ctx (клиент закрыл вкладку, не дождавшись) прекращает ожидание —
// иначе повторные брошенные старты копили бы сеансы-сироты до жнеца.
func (s *Live) awaitHello(ctx context.Context) error {
	wait := s.runner.opts.HelloWait
	deadline := time.After(wait)
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("клиент ушёл, не дождавшись странствия: %w", ctx.Err())
		case line, open := <-s.lines:
			if !open {
				return ErrNoSession
			}
			var hi struct {
				Version int             `json:"eye_session_version"`
				Roots   json.RawMessage `json:"roots"`
			}
			// hello обязан нести МАССИВ корней: пользовательский лог с
			// одним лишь eye_session_version — не рукопожатие, и сеанс с
			// Roots == nil (пустое дерево, некому закрыть) не начинается
			if json.Unmarshal(line, &hi) == nil && hi.Version >= 1 &&
				bytes.HasPrefix(bytes.TrimSpace(hi.Roots), []byte("[")) {
				s.Roots = hi.Roots
				return nil
			}
			// протокольная строка, но не hello — не наш случай, ждём дальше
		case <-s.dead:
			return ErrNoSession
		case <-deadline:
			return fmt.Errorf("%w (рукопожатие не пришло за %s: долгая подготовка перед Explore?)",
				ErrNoSession, wait)
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
	id := s.nextID // локальная копия: сравнение не зависит от чужих инкрементов
	req, _ := json.Marshal(map[string]any{"id": id, "cmd": cmd, "node": node})
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
			if json.Unmarshal(line, &probe) == nil && probe.ID == id {
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
		// quit сериализуется с летящей командой через тот же мьютекс:
		// иначе инкремент nextID под ногами у Do заставил бы её принять
		// ответ quit за свой (или проигнорировать собственный)
		s.mu.Lock()
		if s.stdin != nil {
			s.nextID++
			req, _ := json.Marshal(map[string]any{"id": s.nextID, "cmd": "quit"})
			s.stdin.Write(append(req, '\n'))
		}
		s.mu.Unlock()
		select {
		case <-s.dead:
		case <-time.After(time.Second):
			killProcGroup(s.cmd)
		}
		// группа добивается и при ЧИСТОМ выходе: код странствия мог
		// оставить фоновых детей — им не место после смерти сеанса
		killProcGroup(s.cmd)
		<-s.dead
		os.RemoveAll(s.dir)
		s.runner.unregisterSession(s.ID)
	})
}

// ── реестр сеансов на Runner ─────────────────────────────────────────

// reserveSessionSlot — бронь под будущий сеанс: живые + брони < SessionMax.
// Пока бронь держится (до releaseSessionSlot в конце StartSession), только
// что зарегистрированный сеанс считается дважды — это безопасная сторона
// ошибки: перебор отвергнет лишнего, но никогда не пропустит.
func (r *Runner) reserveSessionSlot() error {
	r.sessMu.Lock()
	defer r.sessMu.Unlock()
	if len(r.sessions)+r.sessPending >= r.opts.SessionMax {
		return fmt.Errorf("%w: живых сеансов уже %d", ErrBusy, len(r.sessions)+r.sessPending)
	}
	r.sessPending++
	return nil
}

func (r *Runner) releaseSessionSlot() {
	r.sessMu.Lock()
	r.sessPending--
	r.sessMu.Unlock()
}

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

// reapLoop — жнец: закрывает простаивающие и зажившиеся сеансы.
func (r *Runner) reapLoop() {
	tick := r.opts.ReapTick
	if tick == 0 {
		tick = reapTick
	}
	for range time.Tick(tick) {
		// снимок указателей под r.sessMu — микросекунды, БЕЗ вложенного
		// s.mu: иначе жнец, ждущий s.mu летящей команды (Do держит его до
		// sessionCmdWait), заморозил бы весь реестр — Session()/register
		// других сеансов встали бы в очередь за r.sessMu (head-of-line)
		r.sessMu.Lock()
		snap := make([]*Live, 0, len(r.sessions))
		for _, s := range r.sessions {
			snap = append(snap, s)
		}
		r.sessMu.Unlock()

		now := time.Now()
		var doomed []*Live
		for _, s := range snap {
			s.mu.Lock()
			idle := now.Sub(s.lastUsed)
			age := now.Sub(s.born)
			s.mu.Unlock()
			if idle > r.opts.SessionIdle || age > r.opts.SessionLife {
				doomed = append(doomed, s)
			}
		}
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
