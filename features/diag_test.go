//go:build acceptance

package features

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/pprof"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

const (
	trailLen    = 20
	pollEvery   = 5 * time.Millisecond
	fakeCLILog  = "SOLODAWN_FAKECLI_LOG"
	artifactsIn = "SOLODAWN_TEST_ARTIFACTS"
)

// trail is what a wait has seen: how many things, and the last ones.
type trail struct {
	mu   sync.Mutex
	n    int
	tail []string
}

func (t *trail) see(what string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.add(what)
}

// read is see for a value that is read again and again: a reading equal to
// the one before is not listed twice.
func (t *trail) read(what string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.n == 0 || t.tail[len(t.tail)-1] != what {
		t.add(what)
	}
}

func (t *trail) add(what string) {
	t.n++
	t.tail = append(t.tail, what)
	if len(t.tail) > trailLen {
		t.tail = append(t.tail[:0], t.tail[1:]...)
	}
}

func (t *trail) timedOut(d time.Duration, what string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return fmt.Errorf("timed out after %s waiting for %s; saw %d: %s", d, what, t.n, strings.Join(t.tail, ", "))
}

// wait is one wait: the time it has, what it is for and what it saw. A
// frame source keeps its own trail, so the failure lists the frames it was
// sent and not only those this wait took; a nil saw starts an empty one.
type wait struct {
	saw   *trail
	d     time.Duration
	what  string
	timer *time.Timer
}

func newWait(d time.Duration, what string, saw *trail) *wait {
	return &wait{saw: cmp.Or(saw, new(trail)), d: d, what: what, timer: time.NewTimer(d)}
}

func (w *wait) stop() { w.timer.Stop() }

func (w *wait) fail() error { return w.saw.timedOut(w.d, w.what) }

// recvWait takes one value from ch, or fails when the time of w is out.
func recvWait[T any](w *wait, ch <-chan T) (T, error) {
	select {
	case v := <-ch:
		return v, nil
	case <-w.timer.C:
		var none T
		return none, w.fail()
	}
}

// waitUntilCtx is waitUntilEvery for the time that ctx has left; a cancelled
// ctx ends it with ctx.Err().
func waitUntilCtx(ctx context.Context, every time.Duration, what string, check func() error) error {
	left := time.Hour
	if deadline, ok := ctx.Deadline(); ok {
		left = time.Until(deadline)
	}
	return waitUntilEvery(every, left, what, func() error {
		if ctx.Err() == context.Canceled {
			return abort(ctx.Err())
		}
		return check()
	})
}

// recvCtx takes one value from ch until ctx ends. A context that ran out
// fails it in the form of a wait, with the time it waited; one that was
// cancelled gives ctx.Err(). saw may be nil.
func recvCtx[T any](ctx context.Context, ch <-chan T, what string, saw *trail) (T, error) {
	start := time.Now()
	select {
	case v := <-ch:
		return v, nil
	case <-ctx.Done():
		var none T
		return none, ctxEnded(ctx, start, what, saw)
	}
}

func ctxEnded(ctx context.Context, start time.Time, what string, saw *trail) error {
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ctx.Err()
	}
	return cmp.Or(saw, new(trail)).timedOut(time.Since(start).Round(100*time.Millisecond), what)
}

// recvFrame takes the next frame from ch within d. The frames taken before
// are listed when it fails; saw is the trail of the source of the frames.
func recvFrame[T any](ch <-chan T, d time.Duration, saw *trail, brief func(T) string) (T, error) {
	w := newWait(d, "a frame", saw)
	defer w.stop()
	v, err := recvWait(w, ch)
	if err == nil {
		saw.see(brief(v))
	}
	return v, err
}

// briefAny is frameBrief for a frame that is still a Go value.
func briefAny[T any](frame T) string {
	raw, err := json.Marshal(frame)
	if err != nil {
		return fmt.Sprintf("%T", frame)
	}
	return frameBrief(raw)
}

func briefString(frame string) string { return frameBrief([]byte(frame)) }

// tailOf is the last 200 bytes of an output, for a reading that is a whole
// terminal screen or log.
func tailOf(s string) string {
	if len(s) <= 200 {
		return s
	}
	return "..." + s[len(s)-200:]
}

// aborted is the error of a check that ends waitUntil at once.
type aborted struct{ error }

func abort(err error) error { return aborted{err} }

// waitUntil calls check every few milliseconds until it returns nil. What
// check returns instead is what it saw: the readings are listed when the wait
// fails. A check that returns abort(err) ends the wait with err.
func waitUntil(d time.Duration, what string, check func() error) error {
	return waitUntilEvery(pollEvery, d, what, check)
}

// waitUntilEvery is waitUntil with a pause between the calls of check.
func waitUntilEvery(every, d time.Duration, what string, check func() error) error {
	w := newWait(d, what, nil)
	defer w.stop()
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		err := check()
		if err == nil {
			return nil
		}
		var stop aborted
		if errors.As(err, &stop) {
			return stop.error
		}
		w.saw.read(err.Error())
		select {
		case <-tick.C:
		case <-w.timer.C:
			return w.fail()
		}
	}
}

// frameBrief names a frame by what a failed wait needs to place it: its type,
// the request it answers, its group and its sequence numbers.
func frameBrief(raw []byte) string {
	var f struct {
		T    string `json:"t"`
		Re   *int64 `json:"re"`
		G    string `json:"g"`
		Seq  *int64 `json:"seq"`
		Rows []struct {
			Seq int64 `json:"seq"`
		} `json:"rows"`
	}
	if json.Unmarshal(raw, &f) != nil {
		return fmt.Sprintf("%.60q", raw)
	}
	parts := []string{"t=" + f.T}
	if f.Re != nil {
		parts = append(parts, fmt.Sprintf("re=%d", *f.Re))
	}
	if f.G != "" {
		parts = append(parts, "g="+f.G)
	}
	if f.Seq != nil {
		parts = append(parts, fmt.Sprintf("seq=%d", *f.Seq))
	}
	if n := len(f.Rows); n > 0 {
		parts = append(parts, fmt.Sprintf("seq=%d-%d", f.Rows[0].Seq, f.Rows[n-1].Seq))
	}
	return strings.Join(parts, " ")
}

// diagBuffer holds what the test process logged during the scenario.
type diagBuffer struct{ syncBuffer }

func (b *diagBuffer) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

// diagChild is a node run as a child process.
type diagChild struct {
	name string
	data string
	cmd  *exec.Cmd
	stop func()
	err  fmt.Stringer
}

var (
	diagLog   diagBuffer
	diagKidsM sync.Mutex
	diagKids  []diagChild
)

// startNodeCmd starts a node as a child process; a scenario that fails
// leaves the node's logs folder and what it wrote to standard error.
func startNodeCmd(t testing.TB, cleanup func(func()), cmd *exec.Cmd, name, data string, stderr fmt.Stringer) (func(), error) {
	stop, err := startManagedCmd(t, cleanup, cmd)
	if err == nil {
		diagKidsM.Lock()
		diagKids = append(diagKids, diagChild{name, data, cmd, stop, stderr})
		diagKidsM.Unlock()
	}
	return stop, err
}

// diagCapture points log at diagLog, at debug level, for the suite; the
// returned function puts the logger back.
func diagCapture() func() {
	out, flags := log.Writer(), log.Flags()
	level := slog.SetLogLoggerLevel(slog.LevelDebug)
	log.SetOutput(&diagLog)
	log.SetFlags(log.Ldate | log.Lmicroseconds | log.LUTC)
	return func() {
		log.SetOutput(out)
		log.SetFlags(flags)
		slog.SetLogLoggerLevel(level)
	}
}

// diagHooks registers the failure dump. Register it before the step sets, so
// that it runs before their clean-up.
func diagHooks(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		diagLog.reset()
		diagKidsM.Lock()
		diagKids = nil
		diagKidsM.Unlock()
		return ctx, nil
	})
	sc.After(func(ctx context.Context, s *godog.Scenario, err error) (context.Context, error) {
		if err == nil {
			return ctx, nil
		}
		dir, werr := diagSave(s.Name)
		return ctx, errors.Join(fmt.Errorf("diagnostics are in %s", dir), werr)
	})
}

// diagSave writes the folder of a failed scenario and returns its path.
func diagSave(scenario string) (string, error) {
	base := cmp.Or(os.Getenv(artifactsIn), os.TempDir())
	name := strings.Map(fileNameRune, scenario)
	if err := os.MkdirAll(base, 0o700); err != nil {
		return base, err
	}
	dir, err := os.MkdirTemp(base, "scenario-"+name[:min(len(name), 40)]+"-")
	if err != nil {
		return base, err
	}
	diagKidsM.Lock()
	kids := slices.Clone(diagKids)
	diagKidsM.Unlock()
	errs := []error{
		os.WriteFile(filepath.Join(dir, "nodes.log"), []byte(diagLog.String()), 0o600),
		diagGoroutines(filepath.Join(dir, "goroutines.txt")),
		diagCopy(os.Getenv(fakeCLILog), filepath.Join(dir, "fakecli.log")),
	}
	for i, c := range kids {
		errs = append(errs, c.save(filepath.Join(dir, fmt.Sprintf("%s-%d", c.name, i))))
	}
	return dir, errors.Join(errs...)
}

func fileNameRune(r rune) rune {
	if strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", r) {
		return r
	}
	return '-'
}

func diagGoroutines(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return errors.Join(pprof.Lookup("goroutine").WriteTo(f, 2), f.Close())
}

// diagCopy copies a file; no file, and no name, are no error.
func diagCopy(from, to string) error {
	raw, err := os.ReadFile(from)
	if from == "" || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return errors.Join(err, os.WriteFile(to, raw, 0o600))
}

// save stops the node, after a SIGQUIT that makes the Go runtime write its
// goroutines to standard error (Windows has no such signal, and no dump), and
// copies what it left.
func (c diagChild) save(dir string) error {
	_ = c.cmd.Process.Signal(syscall.SIGQUIT)
	c.stop()
	logs := os.CopyFS(filepath.Join(dir, "logs"), os.DirFS(filepath.Join(c.data, "logs")))
	if errors.Is(logs, fs.ErrNotExist) {
		logs = nil
	}
	return errors.Join(
		os.MkdirAll(dir, 0o700),
		logs,
		os.WriteFile(filepath.Join(dir, "stderr.txt"), []byte(c.err.String()), 0o600),
		diagCopy(envValue(c.cmd.Env, fakeCLILog), filepath.Join(dir, "fakecli.log")),
	)
}

func envValue(env []string, key string) string {
	for _, entry := range slices.Backward(env) {
		if v, ok := strings.CutPrefix(entry, key+"="); ok {
			return v
		}
	}
	return ""
}
