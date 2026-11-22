package host

import (
	"bufio"
	"context"
	"io"
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

// serverAddress is the URL of a server's `listening on` line. The rest of
// out is discarded.
func serverAddress(out io.Reader) string {
	defer func() { go io.Copy(io.Discard, out) }()
	lines := bufio.NewScanner(out)
	for lines.Scan() {
		if _, url, ok := strings.Cut(lines.Text(), "listening on "); ok {
			return url
		}
	}
	return ""
}

// readSSE calls handle with the data of each event of an event stream.
func readSSE(stream io.Reader, handle func(data []byte)) {
	events := bufio.NewScanner(stream)
	for events.Scan() {
		if data, ok := strings.CutPrefix(events.Text(), "data: "); ok {
			handle([]byte(data))
		}
	}
}
