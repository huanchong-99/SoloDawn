package node

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host"
)

func serveExternalReplies(t *testing.T, lines []string, err error) *[]string {
	t.Helper()
	dir := nodeTestDir(t)
	var (
		mu     sync.Mutex
		opened []string
		wg     sync.WaitGroup
	)
	t.Cleanup(wg.Wait)
	socket := openSocket(t, dir)
	wg.Go(func() {
		for {
			conn, acceptErr := socket.Accept()
			if acceptErr != nil {
				return
			}
			_, line, _, _ := AcceptOpening(conn)
			mu.Lock()
			opened = append(opened, strings.TrimSpace(string(line)))
			mu.Unlock()
			writeLocalReplies(conn, lines, err)
			conn.Close()
		}
	})
	return &opened
}

func runExternal(cmd func([]string, io.Writer, io.Writer) int, args ...string) (int, string, string) {
	var out, errBuf bytes.Buffer
	code := cmd(args, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestExternalCommandArguments(t *testing.T) {
	mustEqual(t, Subcommands["wait"] != nil && Subcommands["done"] != nil, true)
	for _, args := range [][]string{nil, {"--member"}, {"--member", "m1", "x"}, {"--member", "m1", "--timeout", "-2"}, {"--timeout", "1"}} {
		code, _, stderr := runExternal(Wait, args...)
		mustEqual(t, [2]any{code, stderr}, [2]any{2, "usage: solodawn <command> [arguments]\n"})
	}
	for _, args := range [][]string{{"--member", "m1"}, {"--member", "m1", "--run", "0"}, {"--run", "4"}} {
		code, _, _ := runExternal(Done, args...)
		mustEqual(t, code, 2)
	}
}

func TestExternalCommandExits(t *testing.T) {
	opened := serveExternalReplies(t, []string{`{"run":4412}`}, nil)
	code, stdout, _ := runExternal(Wait, "--member", "m1")
	mustEqual(t, [2]any{code, stdout}, [2]any{0, "{\"run\":4412}\n"})
	runExternal(Wait, "--member", "m1", "--timeout", "0")
	code, _, _ = runExternal(Done, "--member", "m1", "--run", "4412", "--failed", "exit 1")
	mustEqual(t, code, 0)
	mustEqual(t, strings.Join(*opened, "\n"), strings.Join([]string{
		`{"cmd":"wait","member":"m1"}`,
		`{"cmd":"wait","member":"m1","timeout":0}`,
		`{"cmd":"done","member":"m1","run":4412,"failed":"exit 1"}`,
	}, "\n"))
}

func TestExternalCommandNoItemAndErrors(t *testing.T) {
	serveExternalReplies(t, nil, nil)
	code, stdout, _ := runExternal(Wait, "--member", "m1", "--timeout", "5")
	mustEqual(t, [2]any{code, stdout}, [2]any{3, ""})
	t.Run("error", func(t *testing.T) {
		serveExternalReplies(t, nil, errors.New("the id names\nno external AI"))
		code, _, stderr := runExternal(Wait, "--member", "m1")
		mustEqual(t, [2]any{code, stderr}, [2]any{1, "the id names no external AI\n"})
		code, _, _ = runExternal(Done, "--member", "m1", "--run", "3")
		mustEqual(t, code, 1)
	})
}

func TestExternalLocalCommands(t *testing.T) {
	h, _, err := host.New(sesOpenStore(t), sesKey("ext-node"), time.Now)
	mustNoErr(t, err)
	t.Cleanup(h.Settle)
	n := &nodeRuntime{ctx: t.Context(), host: h}
	lines, err := n.runLocalCmd(t.Context(), "wait", []byte(`{"cmd":"wait","member":"m1","timeout":0}`))
	mustEqual(t, [2]any{len(lines), err.Error()}, [2]any{0, "not_found: the id names no external AI of this node"})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = n.runLocalCmd(ctx, "wait", []byte(`{"cmd":"wait","member":"m1"}`))
	mustEqual(t, err.Error(), "not_found: the id names no external AI of this node")
	_, err = n.runLocalCmd(t.Context(), "wait", []byte(`{"cmd":"wait","member":1}`))
	mustEqual(t, err != nil, true)
	_, err = n.runLocalCmd(t.Context(), "done", []byte(`{"cmd":"done","member":"m1","run":4}`))
	mustEqual(t, err.Error(), "not_found: the id names no external AI of this node")
	_, err = n.runLocalCmd(t.Context(), "done", []byte(`{"cmd":"done","run":"x"}`))
	mustEqual(t, err != nil, true)
}
