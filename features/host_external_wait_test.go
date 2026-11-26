//go:build acceptance

package features

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"slices"
	"strings"
	"testing/synctest"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type extWaitWorld struct {
	looks   [][]string
	errs    []error
	waited  time.Duration
	tools   []string
	answers []string
	usage   []string
	closed  bool
}

func hostExternalWaitSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &extWaitWorld{}
	sc.Step(`^the runner looks, waits and ends runs of "Scout" by its member id$`, func() error {
		*w = extWaitWorld{}
		return runExtBubble(f.t, w.runWaits)
	})
	sc.Step(`^a look with nothing waiting printed nothing, and after a mention one line of run 1 of kind "group"$`, func() error {
		return errors.Join(w.expectLooks(0, "", `"run":1`), w.expectLooks(1, `"kind":"group"`))
	})
	sc.Step(`^a look while run 1 was open printed nothing, the next look after its done printed run 2, and a late done for run 1 succeeded$`, func() error {
		return errors.Join(w.expectLooks(2, "", `"run":2`), w.expectErrs(0, nil, nil, nil))
	})
	sc.Step(`^a blocking wait printed run 3 after a mention, a wait of 2 seconds printed nothing after 2 seconds, and an unknown id was "not_found"$`, func() error {
		if w.waited != 2*time.Second {
			return fmt.Errorf("waited %v", w.waited)
		}
		return errors.Join(w.expectLooks(4, `"run":3`, ""), w.expectNotFound(3))
	})
	sc.Step(`^a wait open when "Scout" was removed failed with "not_found"$`, func() error { return w.expectNotFound(4) })
	sc.Step(`^a client connects with the member id of "Scout" and calls its tools$`, func() error {
		*w = extWaitWorld{}
		return runExtBubble(f.t, w.runTools)
	})
	sc.Step(`^it was listed the tools of "external" in tools.json and "approve"$`, w.expectTools)
	sc.Step(`^"run_checks" and "submit_review" failed with "conflict" with no run open$`, rewritten("AC-2", "run_checks is removed: the external tools are submit_task by report and submit_review (MODEL-1126 plan 1.8, A10.21)"))
	sc.Step(`^"next_item" answered {"none":true}, then after a mention run 1, and "done" ended that run$`, func() error {
		return w.expectAnswers(2, `{"none":true}`, `"run":1`, `{"ok":true}`)
	})
	sc.Step(`^report_usage 40 with a reset time, then 55, was reported in the member status, and 101 was "bad_request"$`, func() error {
		if !slices.Equal(w.usage, []string{"40 true", "55 false"}) {
			return fmt.Errorf("usage reported %v", w.usage)
		}
		return w.expectAnswers(5, "bad_request:")
	})
	sc.Step(`^a client whose id names no external AI was closed at once$`, func() error {
		if !w.closed {
			return errors.New("the connection stayed open")
		}
		return nil
	})
}

func (w *extWaitWorld) look(b *extBubble, timeout *time.Duration) {
	lines, err := b.h.ExternalWaitLines(context.Background(), hlAI1, timeout)
	w.looks = append(w.looks, lines)
	if err != nil {
		w.errs = append(w.errs, err)
	}
}

func (w *extWaitWorld) runWaits(b *extBubble) error {
	zero := time.Duration(0)
	w.look(b, &zero)
	b.mention(1)
	w.look(b, &zero)
	b.mention(2)
	w.look(b, &zero)
	w.errs = append(w.errs, b.h.ExternalEnd(hlAI1, 1, ""))
	w.look(b, &zero)
	w.errs = append(w.errs, b.h.ExternalEnd(hlAI1, 1, ""), b.h.ExternalEnd(hlAI1, 2, ""))
	waited := make(chan struct{})
	go func() {
		w.look(b, nil)
		close(waited)
	}()
	synctest.Wait()
	b.mention(3)
	<-waited
	b.h.ExternalEnd(hlAI1, 3, "")
	two, start := 2*time.Second, time.Now()
	w.look(b, &two)
	w.waited = time.Since(start)
	w.errs = append(w.errs, b.h.ExternalEnd("unknown", 1, ""))
	removed := make(chan struct{})
	go func() {
		w.look(b, nil)
		close(removed)
	}()
	synctest.Wait()
	gone := fmt.Sprintf(`{"t":"rows","g":%q,"rows":[{"r":"member","id":%q,"seq":99,"gone":true}]}`, hlGroup, hlAI1)
	b.h.Push(b.homeID, json.RawMessage(gone))
	<-removed
	return nil
}

func (w *extWaitWorld) expectLooks(from int, want ...string) error {
	for i, frag := range want {
		got := strings.Join(w.looks[from+i], "\n")
		if frag == "" && got != "" || !strings.Contains(got, frag) {
			return fmt.Errorf("look %d printed %q, want %q", from+i, got, frag)
		}
	}
	return nil
}

func (w *extWaitWorld) expectErrs(from int, want ...error) error {
	for i, e := range want {
		if w.errs[from+i] != e {
			return fmt.Errorf("error %d = %v", from+i, w.errs[from+i])
		}
	}
	return nil
}

func (w *extWaitWorld) expectNotFound(i int) error {
	var e *host.Error
	if len(w.errs) <= i || !errors.As(w.errs[i], &e) || e.Code != "not_found" {
		return fmt.Errorf("errors %v, want not_found at %d", w.errs, i)
	}
	return nil
}

func connectExternal(b *extBubble, member string) (*mcp.ClientSession, error) {
	client, server := net.Pipe()
	go b.h.ServeMCP(context.Background(), member, bufio.NewReader(server), server)
	return mcp.NewClient(&mcp.Implementation{Name: "runner", Version: "1"}, nil).
		Connect(context.Background(), &mcp.IOTransport{Reader: client, Writer: client}, nil)
}

func (w *extWaitWorld) call(cs *mcp.ClientSession, name string, args map[string]any) {
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		panic(err)
	}
	w.answers = append(w.answers, res.Content[0].(*mcp.TextContent).Text)
}

func (w *extWaitWorld) runTools(b *extBubble) error {
	cs, err := connectExternal(b, hlAI1)
	if err != nil {
		return err
	}
	defer cs.Close()
	listed, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		return err
	}
	for _, tool := range listed.Tools {
		w.tools = append(w.tools, tool.Name)
	}
	w.call(cs, "submit_task", map[string]any{"summary": "Done."})
	w.call(cs, "submit_review", map[string]any{"score": 5, "items": []any{}})
	w.call(cs, "next_item", nil)
	b.mention(1)
	w.call(cs, "next_item", nil)
	w.call(cs, "done", map[string]any{"run": 1})
	w.call(cs, "report_usage", map[string]any{"window_pct": 101})
	for _, args := range []map[string]any{{"window_pct": 40, "window_resets": time.Now().Add(3 * time.Hour).UnixMilli()}, {"window_pct": 55}} {
		w.call(cs, "report_usage", args)
		synctest.Wait()
		w.usage = append(w.usage, lastReportedUsage(b))
	}
	_, err = connectExternal(b, "unknown")
	w.closed = err != nil
	return nil
}

func lastReportedUsage(b *extBubble) string {
	cmds := b.home.snapshotCmds()
	for i := len(cmds) - 1; i >= 0; i-- {
		if u, ok := cmds[i]["usage"].(map[string]any); ok && u["window_pct"] != nil {
			return fmt.Sprint(u["window_pct"], " ", u["window_resets"] != nil)
		}
	}
	return ""
}

func (w *extWaitWorld) expectTools() error {
	data, err := os.ReadFile("../internal/proto/testdata/tools.json")
	if err != nil {
		return err
	}
	var lists proto.ToolLists
	if err := json.Unmarshal(data, &lists); err != nil {
		return err
	}
	want := []string{"approve"}
	for _, tool := range lists.External.Tools {
		want = append(want, tool.Name)
	}
	slices.Sort(want)
	got := slices.Sorted(slices.Values(w.tools))
	if !slices.Equal(got, want) {
		return fmt.Errorf("listed %v, want %v", got, want)
	}
	return nil
}

func (w *extWaitWorld) expectAnswers(from int, want ...string) error {
	for i, frag := range want {
		if !strings.Contains(w.answers[from+i], frag) {
			return fmt.Errorf("answer %d = %q, want %q", from+i, w.answers[from+i], frag)
		}
	}
	return nil
}
