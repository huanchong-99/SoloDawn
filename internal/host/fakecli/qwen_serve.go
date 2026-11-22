package fakecli

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
)

type qwenScript struct {
	Session    string     `json:"session"`
	BootStderr string     `json:"bootStderr,omitempty"`
	BootExit   int        `json:"bootExit,omitempty"`
	OpenStatus int        `json:"openStatus,omitempty"`
	OpenBody   any        `json:"openBody,omitempty"`
	Steps      []qwenStep `json:"steps"`
	Exit       int        `json:"exit,omitempty"`
}

type qwenStep struct {
	Dir string          `json:"dir"`
	Msg json.RawMessage `json:"msg,omitempty"`
}

type qwenDaemon struct {
	token   string
	script  qwenScript
	session json.RawMessage
	stderr  io.Writer
	proc    *process
	// Single reader: nextInbound in qwenDaemon.
	inbound chan []byte
	// Single reader: serveSSE in qwenDaemon.
	sseOut chan []byte
	// Closed once when GET /session/:id/events attaches.
	sseReady  chan struct{}
	readyOnce sync.Once
	// Closed once on DELETE /session/:id, stdin EOF or SIGTERM.
	stopCh   chan struct{}
	stopOnce sync.Once
}

// RunQwen runs one start of the fake Qwen Code CLI (`--version` or `serve`).
func RunQwen(version string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if slices.Equal(args, []string{"--version"}) {
		fmt.Fprintln(stdout, version)
		return 0
	}
	code, err := runQwenServe(args, stdin, stdout, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "fake qwen:", err)
		return 1
	}
	return code
}

func runQwenServe(args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if len(args) == 0 || args[0] != "serve" {
		return 0, errors.New("expected serve subcommand")
	}
	s, err := loadQwenScript()
	if err != nil {
		return 0, err
	}
	if s.BootExit != 0 {
		fmt.Fprintln(stderr, s.BootStderr)
		return s.BootExit, nil
	}
	if s.Session == "" {
		return 0, errors.New("no session id in script")
	}
	return serveQwenScript(s, args, stdin, stdout, stderr)
}

func loadQwenScript() (qwenScript, error) {
	var s qwenScript
	data, err := os.ReadFile(os.Getenv("SOLODAWN_FAKE_QWEN_SCRIPT"))
	if err == nil {
		err = json.Unmarshal(data, &s)
	}
	if err != nil {
		return s, fmt.Errorf("SOLODAWN_FAKE_QWEN_SCRIPT: %w", err)
	}
	return s, nil
}

func serveQwenScript(s qwenScript, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	logFile, err := os.OpenFile(os.Getenv("SOLODAWN_FAKECLI_LOG"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return 0, fmt.Errorf("SOLODAWN_FAKECLI_LOG: %w", err)
	}
	defer logFile.Close()
	fmt.Fprintln(logFile, "start qwen", s.Session)
	defer fmt.Fprintln(logFile, "exit qwen", s.Session)

	d := newQwenDaemon(s, value(args, "--token"), stderr)
	defer d.proc.stopMCP()
	ln, srv, err := d.listenHTTP()
	if err != nil {
		return 0, err
	}
	defer srv.Shutdown(context.Background())
	watchQwenStopSignals(stdin, d.triggerStop)
	fmt.Fprintf(stdout, "qwen serve listening on http://%s (mode=http-bridge, workspace=%s)\n", ln.Addr().String(), value(args, "--workspace"))
	return d.runSteps()
}

func newQwenDaemon(s qwenScript, token string, stderr io.Writer) *qwenDaemon {
	sessJSON, _ := json.Marshal(s.Session)
	settingsPath := os.Getenv("QWEN_CODE_SYSTEM_SETTINGS_PATH")
	return &qwenDaemon{
		token:    token,
		script:   s,
		session:  sessJSON,
		stderr:   stderr,
		proc:     &process{vendor: "qwen", args: []string{"--mcp-config", settingsPath}, stderr: stderr},
		inbound:  make(chan []byte, 16),
		sseOut:   make(chan []byte, 32),
		sseReady: make(chan struct{}),
		stopCh:   make(chan struct{}),
	}
}

func watchQwenStopSignals(stdin io.Reader, stop func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, os.Interrupt)
	go func() {
		select {
		case <-sigCh:
			stop()
		}
		signal.Stop(sigCh)
	}()
	go func() {
		_, _ = io.Copy(io.Discard, stdin)
		stop()
	}()
}

func (d *qwenDaemon) triggerStop() {
	d.stopOnce.Do(func() { close(d.stopCh) })
}

func (d *qwenDaemon) listenHTTP() (net.Listener, *http.Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	srv := &http.Server{Handler: http.HandlerFunc(d.serveHTTP)}
	go srv.Serve(ln)
	return ln, srv, nil
}

func (d *qwenDaemon) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if d.token != "" && r.Header.Get("Authorization") != "Bearer "+d.token {
		writeQwenJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}
	d.routeHTTP(w, r)
}

func (d *qwenDaemon) routeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/session":
		d.handleCreateSession(w, r)
	case len(parts) == 2 && parts[0] == "session" && r.Method == http.MethodDelete:
		d.triggerStop()
		w.WriteHeader(http.StatusNoContent)
	case len(parts) >= 3 && parts[0] == "session":
		d.routeSessionSubpath(w, r, parts[1], parts[2:])
	default:
		writeQwenJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (d *qwenDaemon) routeSessionSubpath(w http.ResponseWriter, r *http.Request, sid string, tail []string) {
	switch tail[0] {
	case "resume":
		d.handleResumeSession(w, r, sid)
	case "approval-mode":
		writeQwenJSON(w, http.StatusOK, map[string]any{"sessionId": sid, "mode": "yolo", "previous": "default", "persisted": false})
	case "events":
		d.serveSSE(w, r)
	case "prompt":
		d.handlePrompt(w, r, sid)
	case "cancel":
		d.pushInbound(map[string]any{"route": "cancel", "sessionId": sid})
		w.WriteHeader(http.StatusNoContent)
	case "permission":
		d.handlePermissionVote(w, r, sid, tail)
	}
}

func (d *qwenDaemon) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if d.script.OpenStatus >= 400 {
		writeQwenJSON(w, d.script.OpenStatus, cmp.Or[any](d.script.OpenBody, map[string]string{"error": "open failed"}))
		d.triggerStop()
		return
	}
	var req struct {
		Cwd string `json:"cwd"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	writeQwenJSON(w, http.StatusOK, map[string]any{
		"sessionId": d.script.Session, "workspaceCwd": req.Cwd, "attached": false,
	})
}

func (d *qwenDaemon) handleResumeSession(w http.ResponseWriter, r *http.Request, sid string) {
	if d.script.OpenStatus >= 400 || sid != d.script.Session {
		status := cmp.Or(d.script.OpenStatus, http.StatusNotFound)
		body := cmp.Or[any](d.script.OpenBody, map[string]string{"code": "session_not_found", "error": "Session not found"})
		writeQwenJSON(w, status, body)
		d.triggerStop()
		return
	}
	var req struct {
		Cwd string `json:"cwd"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	writeQwenJSON(w, http.StatusOK, map[string]any{
		"sessionId": sid, "workspaceCwd": req.Cwd, "attached": false, "state": map[string]any{},
	})
}

func (d *qwenDaemon) handlePrompt(w http.ResponseWriter, r *http.Request, sid string) {
	var body any
	_ = json.NewDecoder(r.Body).Decode(&body)
	d.pushInbound(map[string]any{"route": "prompt", "sessionId": sid, "body": body})
	writeQwenJSON(w, http.StatusAccepted, map[string]any{"promptId": sid + "########1", "lastEventId": 0})
}

func (d *qwenDaemon) handlePermissionVote(w http.ResponseWriter, r *http.Request, sid string, tail []string) {
	var body any
	_ = json.NewDecoder(r.Body).Decode(&body)
	reqID := ""
	if len(tail) > 1 {
		reqID = tail[1]
	}
	d.pushInbound(map[string]any{"route": "permission", "sessionId": sid, "requestId": reqID, "body": body})
	writeQwenJSON(w, http.StatusOK, map[string]any{})
}

func (d *qwenDaemon) pushInbound(v any) {
	raw, _ := json.Marshal(v)
	d.inbound <- raw
}

func (d *qwenDaemon) serveSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	flusher.Flush()
	d.readyOnce.Do(func() { close(d.sseReady) })
	for {
		select {
		case frame, ok := <-d.sseOut:
			if !ok {
				return
			}
			_, _ = w.Write(frame)
			flusher.Flush()
		case <-r.Context().Done():
			return
		case <-d.stopCh:
			return
		}
	}
}

func writeQwenJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (d *qwenDaemon) runSteps() (int, error) {
	defer close(d.sseOut)
	for i, step := range d.script.Steps {
		if err := d.execStep(step); err != nil {
			return 0, fmt.Errorf("step %d (%s): %w", i+1, step.Dir, err)
		}
	}
	return d.script.Exit, nil
}

func (d *qwenDaemon) execStep(st qwenStep) error {
	msg := bytes.ReplaceAll(st.Msg, []byte(`"$session"`), d.session)
	switch st.Dir {
	case "http<-node":
		return d.expectInbound(msg)
	case "sse->node":
		return d.emitSSE(msg)
	case "mcp<-cli", "mcp->cli":
		return d.proc.mcpStep(st.Dir, msg)
	case "stderr":
		return d.proc.print(msg)
	case "eof":
		<-d.stopCh
		return nil
	default:
		return fmt.Errorf("unknown direction %q", st.Dir)
	}
}

func (d *qwenDaemon) expectInbound(want []byte) error {
	select {
	case raw := <-d.inbound:
		return matchQwenJSON(raw, want)
	case <-d.stopCh:
		return errors.New("stopped before expected HTTP request")
	}
}

func matchQwenJSON(raw, want []byte) error {
	var got, fields any
	if err := errors.Join(json.Unmarshal(raw, &got), json.Unmarshal(want, &fields)); err != nil {
		return err
	}
	if !contains(got, fields) {
		return fmt.Errorf("expected %s, got %s", want, raw)
	}
	return nil
}

func (d *qwenDaemon) emitSSE(msg []byte) error {
	<-d.sseReady
	var hdr struct {
		ID   *int64 `json:"id"`
		Type string `json:"type"`
	}
	var compact bytes.Buffer
	if err := errors.Join(json.Unmarshal(msg, &hdr), json.Compact(&compact, msg)); err != nil {
		return err
	}
	var frame bytes.Buffer
	if hdr.ID != nil {
		fmt.Fprintf(&frame, "id: %d\n", *hdr.ID)
	}
	fmt.Fprintf(&frame, "event: %s\ndata: %s\n\n", hdr.Type, compact.Bytes())
	d.sseOut <- frame.Bytes()
	return nil
}
