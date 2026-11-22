package fakecli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

type openCodeScript struct {
	Session       string         `json:"session"`
	StartupStderr string         `json:"startupStderr"`
	Steps         []openCodeStep `json:"steps"`
	Exit          int            `json:"exit"`
}

type openCodeStep struct {
	Dir    string          `json:"dir"`
	Req    *openCodeExpect `json:"req"`
	Status int             `json:"status"`
	Res    json.RawMessage `json:"res"`
	Msg    json.RawMessage `json:"msg"`
}

type openCodeExpect struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body"`
}

type openCodeHTTPExchange struct {
	method string
	path   string
	body   []byte
	reply  chan openCodeHTTPReply
}

type openCodeHTTPReply struct {
	status int
	body   []byte
}

type openCodeSSEItem struct {
	payload []byte
	flushed chan struct{}
}

type openCodeServer struct {
	proc      *process
	password  string
	requests  chan openCodeHTTPExchange
	disposed  chan struct{}
	dispOnce  sync.Once
	sseReady  chan struct{}
	readyOnce sync.Once

	mu      sync.Mutex
	streams map[chan openCodeSSEItem]struct{}
}

func (p *process) replayOpenCode(rawStdin io.Reader) (int, error) {
	s, err := loadOpenCodeScript()
	if err != nil {
		return 0, err
	}
	if s.Session == "" {
		return 0, errors.New("no session id in the script")
	}
	log, err := os.OpenFile(os.Getenv("SOLODAWN_FAKECLI_LOG"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return 0, fmt.Errorf("SOLODAWN_FAKECLI_LOG: %w", err)
	}
	defer log.Close()
	fmt.Fprintln(log, "start", p.vendor, s.Session)
	defer fmt.Fprintln(log, "exit", p.vendor, s.Session)
	defer p.stopMCP()
	if s.StartupStderr != "" {
		fmt.Fprintln(p.stderr, s.StartupStderr)
		return s.Exit, nil
	}
	p.session, _ = json.Marshal(s.Session)
	return p.serveOpenCodeSteps(s, rawStdin)
}

func loadOpenCodeScript() (s openCodeScript, err error) {
	const key = "SOLODAWN_FAKE_OPENCODE_SCRIPT"
	data, err := os.ReadFile(os.Getenv(key))
	if err == nil {
		err = json.Unmarshal(data, &s)
	}
	if err != nil {
		err = fmt.Errorf("%s: %w", key, err)
	}
	return s, err
}

func (p *process) serveOpenCodeSteps(s openCodeScript, rawStdin io.Reader) (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	srv := &openCodeServer{
		proc:     p,
		password: os.Getenv("OPENCODE_SERVER_PASSWORD"),
		requests: make(chan openCodeHTTPExchange),
		disposed: make(chan struct{}),
		sseReady: make(chan struct{}),
		streams:  map[chan openCodeSSEItem]struct{}{},
	}
	httpSrv := &http.Server{Handler: srv}
	defer func() {
		srv.markDisposed()
		srv.closeStreams()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		httpSrv.Shutdown(ctx)
		cancel()
		httpSrv.Close()
	}()
	go httpSrv.Serve(ln)
	stopWatch := srv.watchSignalsAndStdin(rawStdin)
	defer stopWatch()
	fmt.Fprintf(p.stdout, "opencode server listening on http://%s\n", ln.Addr().String())
	for i, st := range s.Steps {
		if err := srv.runStep(st); err != nil {
			return 0, fmt.Errorf("step %d (%s): %w", i+1, st.Dir, err)
		}
	}
	return s.Exit, nil
}

func (s *openCodeServer) watchSignalsAndStdin(rawStdin io.Reader) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	stopCh := make(chan struct{})
	go func() {
		select {
		case <-sigCh:
			s.markDisposed()
		case <-stopCh:
		}
	}()
	go func() {
		io.Copy(io.Discard, rawStdin)
		s.markDisposed()
	}()
	return func() {
		signal.Stop(sigCh)
		close(stopCh)
	}
}

func (s *openCodeServer) markDisposed() {
	s.dispOnce.Do(func() { close(s.disposed) })
}

func (s *openCodeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, `{"name":"UnauthorizedError","data":{"message":"unauthorized"}}`, http.StatusUnauthorized)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/event" {
		s.serveSSE(w, r.Context())
		return
	}
	if r.Method == http.MethodPost && (r.URL.Path == "/global/dispose" || r.URL.Path == "/instance/dispose") {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, "true")
		s.markDisposed()
		return
	}
	body, _ := io.ReadAll(r.Body)
	replyCh := make(chan openCodeHTTPReply, 1)
	select {
	case s.requests <- openCodeHTTPExchange{method: r.Method, path: r.URL.Path, body: body, reply: replyCh}:
	case <-r.Context().Done():
		return
	case <-s.disposed:
		http.Error(w, "disposed", http.StatusServiceUnavailable)
		return
	}
	rep, ok := <-replyCh
	if !ok {
		http.Error(w, "step failed", http.StatusInternalServerError)
		return
	}
	if len(rep.body) > 0 {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(rep.status)
	w.Write(rep.body)
}

func (s *openCodeServer) authorized(r *http.Request) bool {
	if s.password == "" {
		return true
	}
	user, pass, ok := r.BasicAuth()
	return ok && user == "opencode" && pass == s.password
}

func (s *openCodeServer) serveSSE(w http.ResponseWriter, ctx context.Context) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	io.WriteString(w, "data: {\"type\":\"server.connected\",\"properties\":{}}\n\n")
	flusher.Flush()
	ch := make(chan openCodeSSEItem, 32)
	s.addStream(ch)
	defer s.removeStream(ch)
	s.readyOnce.Do(func() { close(s.sseReady) })
	for {
		select {
		case item, open := <-ch:
			if !open {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", item.payload)
			flusher.Flush()
			close(item.flushed)
		case <-ctx.Done():
			return
		}
	}
}

func (s *openCodeServer) addStream(ch chan openCodeSSEItem) {
	s.mu.Lock()
	s.streams[ch] = struct{}{}
	s.mu.Unlock()
}

func (s *openCodeServer) removeStream(ch chan openCodeSSEItem) {
	s.mu.Lock()
	delete(s.streams, ch)
	s.mu.Unlock()
}

func (s *openCodeServer) closeStreams() {
	s.mu.Lock()
	for ch := range s.streams {
		close(ch)
		delete(s.streams, ch)
	}
	s.mu.Unlock()
}

func (s *openCodeServer) broadcastSSE(raw []byte) error {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return err
	}
	select {
	case <-s.sseReady:
	case <-s.disposed:
		return errors.New("disposed before SSE connected")
	}
	payload := compact.Bytes()
	var waits []chan struct{}
	s.mu.Lock()
	for ch := range s.streams {
		flushed := make(chan struct{})
		ch <- openCodeSSEItem{payload: payload, flushed: flushed}
		waits = append(waits, flushed)
	}
	s.mu.Unlock()
	for _, w := range waits {
		select {
		case <-w:
		case <-s.disposed:
			return nil
		}
	}
	return nil
}

func (s *openCodeServer) runStep(st openCodeStep) error {
	unquotedSession := strings.Trim(string(s.proc.session), `"`)
	msg := bytes.ReplaceAll(st.Msg, []byte(`"$session"`), s.proc.session)
	switch st.Dir {
	case "http":
		return s.expectHTTP(st, unquotedSession)
	case "sse":
		return s.broadcastSSE(msg)
	case "mcp<-cli", "mcp->cli":
		return s.proc.mcpStep(st.Dir, msg)
	case "stderr":
		return s.proc.print(msg)
	case "eof", "dispose":
		<-s.disposed
		return nil
	default:
		return fmt.Errorf("unknown direction %q", st.Dir)
	}
}

func (s *openCodeServer) expectHTTP(st openCodeStep, session string) error {
	if st.Req == nil {
		return errors.New("missing http req")
	}
	var ex openCodeHTTPExchange
	select {
	case ex = <-s.requests:
	case <-s.disposed:
		return errors.New("disposed while waiting for http request")
	}
	wantPath := strings.ReplaceAll(st.Req.Path, "$session", session)
	if err := matchOpenCodeRequest(ex, st.Req.Method, wantPath, s.replaceSession(st.Req.Body)); err != nil {
		close(ex.reply)
		return err
	}
	status := st.Status
	if status == 0 {
		status = http.StatusOK
	}
	ex.reply <- openCodeHTTPReply{status: status, body: s.replaceSession(st.Res)}
	return nil
}

func (s *openCodeServer) replaceSession(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	return bytes.ReplaceAll(raw, []byte(`"$session"`), s.proc.session)
}

func matchOpenCodeRequest(ex openCodeHTTPExchange, wantMethod, wantPath string, wantBody []byte) error {
	if ex.method != wantMethod || ex.path != wantPath {
		return fmt.Errorf("expected %s %s, got %s %s", wantMethod, wantPath, ex.method, ex.path)
	}
	if len(wantBody) == 0 {
		return nil
	}
	var got, want any
	if err := errors.Join(json.Unmarshal(ex.body, &got), json.Unmarshal(wantBody, &want)); err != nil {
		return err
	}
	if !contains(got, want) {
		return fmt.Errorf("expected body %s, got %s", wantBody, ex.body)
	}
	return nil
}
