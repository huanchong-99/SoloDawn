//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const externalRunnerScript = `S="$EXTERNAL_SOLODAWN"; M="$EXTERNAL_MEMBER"
item=$("$S" wait --member "$M" %s) || exit $?
run=$(printf '%%s' "$item" | jq -r .run)
dir=$(printf '%%s' "$item" | jq -r .folder)
printf '%%s\n' "$item" > "$EXTERNAL_RUN_DIR/$run.json"
if (cd "$dir" && printf '%%s' "$item" |
    jq -r '.context + "\n\n" + .text' | "$EXTERNAL_FAKE_AGENT" "$run"); then
  "$S" done --member "$M" --run "$run"
else
  "$S" done --member "$M" --run "$run" --failed "exit $?"
  exit 1
fi
`

type externalAgentStart struct {
	Run, Folder, Input string
}

func init() {
	if os.Getenv("EXTERNAL_AGENT_GATE") == "" {
		return
	}
	if err := runExternalFakeAgent(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func runExternalFakeAgent() error {
	dir := os.Getenv("EXTERNAL_RUN_DIR")
	if err := logExternalAgentStart(dir, os.Args[1]); err != nil {
		return err
	}
	lock := filepath.Join(dir, "agent-active")
	if err := os.Mkdir(lock, 0o700); err != nil {
		return fmt.Errorf("external agents overlapped: %w", err)
	}
	defer os.Remove(lock)
	answer, err := externalAgentAnswer(os.Args[1])
	if err != nil {
		return err
	}
	return externalAgentPost(answer)
}

func logExternalAgentStart(dir, run string) error {
	folder, err := os.Getwd()
	if err != nil {
		return err
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(dir, "starts.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	return json.NewEncoder(log).Encode(externalAgentStart{run, folder, string(input)})
}

func externalAgentAnswer(run string) (string, error) {
	client := http.Client{Timeout: 30 * time.Second}
	res, err := client.Post(os.Getenv("EXTERNAL_AGENT_GATE"), "text/plain", strings.NewReader(run))
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gate returned %s", res.Status)
	}
	body, err := io.ReadAll(res.Body)
	return string(body), err
}

func externalAgentPost(answer string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Getenv("EXTERNAL_SOLODAWN"), "mcp", "--member", os.Getenv("EXTERNAL_MEMBER"))
	client := mcp.NewClient(&mcp.Implementation{Name: "external-scripted-agent", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return err
	}
	defer session.Close()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "post", Arguments: map[string]string{"text": answer}})
	if err != nil {
		return err
	}
	if res.IsError {
		return fmt.Errorf("scripted external post failed: %+v", res)
	}
	return nil
}

type externalRunner struct {
	ctx    context.Context
	dir    string
	env    []string
	ready  chan string
	answer chan string
	looks  chan externalScheduledLook
}

type externalScheduledLook struct {
	externalCLIResult
	started time.Time
}

func (w *externalWorld) runQueued(mode string, count int) error {
	r, err := w.startExternalRunner(mode)
	if err != nil {
		return err
	}
	if count == 1 {
		return w.observeRunner(r, [][2]string{{"Setup check: answer here in one line, then finish.", "Setup check answered"}})
	}
	return w.observeRunner(r, [][2]string{
		{"First external mention", "First external answer"},
		{"Second external mention", "Second external answer"},
	})
}

func (w *externalWorld) startExternalRunner(mode string) (*externalRunner, error) {
	ctx, cancel := context.WithTimeout(w.ctx, 45*time.Second)
	r := &externalRunner{ctx: ctx, dir: filepath.Join(w.root, "runner"), ready: make(chan string), answer: make(chan string)}
	server := httptest.NewServer(http.HandlerFunc(r.gate))
	registerBothCleanups(w.f.t, w.cleanup, func() { cancel(); server.Close() })
	if err := os.Mkdir(r.dir, 0o700); err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	r.env = append(w.command().Env, "EXTERNAL_RUN_DIR="+r.dir, "EXTERNAL_AGENT_GATE="+server.URL,
		"EXTERNAL_SOLODAWN="+w.f.bin, "EXTERNAL_MEMBER="+w.aiByName["X"].ID, "EXTERNAL_FAKE_AGENT="+exe)
	return r, r.start(mode, &w.m6World)
}

func (r *externalRunner) gate(out http.ResponseWriter, req *http.Request) {
	run, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(out, err.Error(), http.StatusBadRequest)
		return
	}
	select {
	case r.ready <- string(run):
	case <-r.ctx.Done():
		return
	}
	select {
	case answer := <-r.answer:
		fmt.Fprint(out, answer)
	case <-r.ctx.Done():
		http.Error(out, r.ctx.Err().Error(), http.StatusRequestTimeout)
	}
}

func (r *externalRunner) start(mode string, w *m6World) error {
	timeout := "--timeout 0"
	if mode == "loop" {
		timeout = ""
	}
	script := filepath.Join(r.dir, "runner.sh")
	if err := os.WriteFile(script, fmt.Appendf(nil, externalRunnerScript, timeout), 0o600); err != nil {
		return err
	}
	if mode == "loop" {
		cmd := exec.CommandContext(r.ctx, "sh", "-c", `while :; do sh "$1" || sleep 1; done`, "external-runner", script)
		cmd.Env = r.env
		_, err := startManagedCmd(w.f.t, w.cleanup, cmd)
		return err
	}
	r.looks = make(chan externalScheduledLook, 8)
	go r.schedule(script)
	return nil
}

func (r *externalRunner) schedule(script string) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			go r.look(script)
		case <-r.ctx.Done():
			return
		}
	}
}

func (r *externalRunner) look(script string) {
	cmd := exec.CommandContext(r.ctx, "sh", script)
	cmd.Env = r.env
	started := time.Now()
	result := externalScheduledLook{runExternalCLI(cmd), started}
	select {
	case r.looks <- result:
	case <-r.ctx.Done():
	}
}

func (w *externalWorld) observeRunner(r *externalRunner, items [][2]string) error {
	for i, item := range items {
		if err := w.observeRunnerItem(r, i+1, item); err != nil {
			return err
		}
	}
	return s3Run(func() error { return w.state("X", "waiting") },
		r.expectEmptyLook,
		func() error { _, err := r.starts(len(items)); return err })
}

func (w *externalWorld) observeRunnerItem(r *externalRunner, count int, item [2]string) error {
	run, err := r.nextStart()
	if err != nil {
		return err
	}
	return s3Run(func() error { return w.checkRunnerInput(r, run, count, item[0]) },
		func() error { return w.state("X", "working") },
		r.expectEmptyLook,
		func() error { return r.finish(item[1]) },
		func() error { return w.externalPost(item[1]) })
}

func (r *externalRunner) nextStart() (string, error) {
	start := time.Now()
	for {
		select {
		case run := <-r.ready:
			return run, nil
		case result := <-r.looks:
			if err := result.expect(3); err != nil {
				return "", err
			}
		case <-r.ctx.Done():
			return "", ctxEnded(r.ctx, start, "the external agent to start", nil)
		}
	}
}

func (r *externalRunner) expectEmptyLook() error {
	return r.expectLook(3, time.Now())
}

func (r *externalRunner) expectLook(code int, since time.Time) error {
	if r.looks == nil {
		return nil
	}
	start := time.Now()
	for {
		select {
		case result := <-r.looks:
			if result.earlierEmptyLook(code, since) {
				continue
			}
			return result.expect(code)
		case <-r.ctx.Done():
			return ctxEnded(r.ctx, start, fmt.Sprintf("a look with exit code %d", code), nil)
		}
	}
}

func (result externalScheduledLook) earlierEmptyLook(code int, since time.Time) bool {
	return result.expect(3) == nil && (code == 0 || result.started.Before(since))
}

func (r *externalRunner) finish(answer string) error {
	start := time.Now()
	select {
	case r.answer <- answer:
		return r.expectLook(0, time.Time{})
	case <-r.ctx.Done():
		return ctxEnded(r.ctx, start, "the runner to take the answer", nil)
	}
}

func (r *externalRunner) starts(want int) (externalAgentStart, error) {
	raw, err := os.ReadFile(filepath.Join(r.dir, "starts.jsonl"))
	if err != nil {
		return externalAgentStart{}, err
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != want {
		return externalAgentStart{}, fmt.Errorf("external agent starts = %d, want %d", len(lines), want)
	}
	var got externalAgentStart
	err = json.Unmarshal([]byte(lines[want-1]), &got)
	return got, err
}

func (w *externalWorld) checkRunnerInput(r *externalRunner, run string, count int, text string) error {
	raw, err := os.ReadFile(filepath.Join(r.dir, run+".json"))
	if err != nil {
		return err
	}
	if err := s3Run(func() error { return w.decodeHandout(raw) }, func() error { return w.handout("group", text) }); err != nil {
		return err
	}
	start, err := r.starts(count)
	if err != nil {
		return err
	}
	want := externalAgentStart{run, w.run.Folder, w.run.Context + "\n\n" + w.run.Text + "\n"}
	if start != want {
		return fmt.Errorf("external agent start = %+v, want %+v", start, want)
	}
	return nil
}
