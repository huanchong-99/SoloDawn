package host

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// serve starts a CLI's local server in folder until ctx ends. It returns the
// server's standard output and wait, which waits for the server once ctx
// ended and then kills what it left running.
func serve(ctx context.Context, folder string, env []string, name string, args ...string) (*os.File, func(), error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = folder
	cmd.Env = append(os.Environ(), env...)
	detachTerminal(cmd)
	cmd.WaitDelay = time.Second
	out, w, _ := os.Pipe()
	cmd.Stdout = w
	err := cmd.Start()
	w.Close()
	if err != nil {
		out.Close()
		return nil, nil, err
	}
	kill := killCheckTree(cmd)
	return out, func() {
		cmd.Wait()
		kill()
		out.Close()
	}, nil
}

// serverAddress is the URL of a server's `listening on` line, up to the
// first space after it. The rest of out is discarded.
func serverAddress(out io.Reader) string {
	defer func() { go io.Copy(io.Discard, out) }()
	lines := bufio.NewScanner(out)
	for lines.Scan() {
		if _, url, ok := strings.Cut(lines.Text(), "listening on "); ok {
			url, _, _ = strings.Cut(url, " ")
			return url
		}
	}
	return ""
}

// wireBody is the JSON body of a request to a CLI's server. A request without
// a value has none: a GET with a body loses its event stream when a Go reverse
// proxy forwards it (issues.md, 2026-11-26).
func wireBody(v any) io.Reader {
	if v == nil {
		return http.NoBody
	}
	return strings.NewReader(marshalWire(v))
}

// readSSE calls handle with the data of each event of an event stream. It
// reads with bufio.Reader, so an event has no size limit.
func readSSE(stream io.Reader, handle func(data []byte)) {
	events := bufio.NewReader(stream)
	for {
		line, err := events.ReadString('\n')
		if err != nil {
			return
		}
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			handle([]byte(strings.TrimSpace(data)))
		}
	}
}
