package fakecli

import (
	"bufio"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
)

// A fake OpenCode serves its script over the HTTP API of `opencode serve`:
// "http" takes the next request and answers it, "sse" sends an event on the
// event stream.
type openCode struct {
	token string // Qwen Code's bearer token; OpenCode uses a basic-auth password
	calls chan openCodeCall
	sse   *io.PipeReader
	send  *io.PipeWriter
}

type openCodeCall struct {
	call, body string
	reply      chan openCodeReply
}

type openCodeReply struct {
	status int
	body   []byte
}

func (p *process) listen() {
	p.http = &openCode{token: value(p.args, "--token"), calls: make(chan openCodeCall)}
	p.http.sse, p.http.send = io.Pipe()
	srv := httptest.NewServer(p.http)
	fmt.Fprintln(p.stdout, p.vendor+" server listening on "+srv.URL)
}

func (o *openCode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !o.authorized(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	// OpenCode streams on /event, Qwen Code on /session/<id>/events.
	if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "s"), "/event") {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		lines := bufio.NewScanner(o.sse)
		for lines.Scan() {
			fmt.Fprintf(w, "data: %s\n\n", lines.Bytes())
			w.(http.Flusher).Flush()
		}
		return
	}
	body, _ := io.ReadAll(r.Body)
	reply := make(chan openCodeReply)
	o.calls <- openCodeCall{r.Method + " " + r.URL.Path, string(body), reply}
	res := <-reply
	w.WriteHeader(res.status)
	w.Write(res.body)
}

func (o *openCode) authorized(r *http.Request) bool {
	if o.token != "" {
		return r.Header.Get("Authorization") == "Bearer "+o.token
	}
	_, password, _ := r.BasicAuth()
	return password == os.Getenv("OPENCODE_SERVER_PASSWORD")
}

func (p *process) httpStep(dir string, msg []byte) error {
	switch dir {
	case "http":
		return p.call(msg)
	case "sse":
		return send(p.http.send, msg)
	}
	return fmt.Errorf("unknown direction %q", dir)
}

// call checks the next request against the method, path and fields of want.
func (p *process) call(msg []byte) error {
	var want struct {
		Call   string
		Body   json.RawMessage
		Status int
		Res    json.RawMessage
	}
	if err := json.Unmarshal(msg, &want); err != nil {
		return err
	}
	got := <-p.http.calls
	if got.call != want.Call {
		return fmt.Errorf("expected %s, got %s", want.Call, got.call)
	}
	if err := match([]byte(cmp.Or(got.body, "null")), want.Body); err != nil {
		return err
	}
	got.reply <- openCodeReply{cmp.Or(want.Status, http.StatusOK), want.Res}
	return nil
}
