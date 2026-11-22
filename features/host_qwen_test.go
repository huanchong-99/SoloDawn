//go:build acceptance

package features

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
)

func init() {
	stepSets["host_qwen"] = hostQwenSteps
}

type qwenAcceptance struct {
	f      fixtures
	undo   func(func())
	log    string
	events []host.Event
}

func hostQwenSteps(sc *godog.ScenarioContext, f fixtures) {
	q := &qwenAcceptance{f: f, undo: afterScenario(sc)}
	sc.Step(`^a scripted Qwen Code daemon that asks to run "([^"]+)" in the first of two turns$`, func(command string) error {
		return q.script(`{"session":"qs-accept","steps":[
			{"dir":"http","msg":{"call":"POST /session","body":{},"res":{"sessionId":"$session"}}},
			{"dir":"http","msg":{"call":"POST /session/qs-accept/prompt","body":{"prompt":[{"type":"text","text":"Run the tests."}]}}},
			{"dir":"sse","msg":{"type":"permission_request","data":{"requestId":"req-1","toolCall":{"kind":"execute","rawInput":{"command":"` + command + `"}}}}},
			{"dir":"http","msg":{"call":"POST /session/qs-accept/permission/req-1","body":{"outcome":{"outcome":"selected","optionId":"cancel"}}}},
			{"dir":"sse","msg":{"type":"turn_complete","data":{"stopReason":"end_turn"}}},
			{"dir":"http","msg":{"call":"POST /session/qs-accept/prompt","body":{"prompt":[{"type":"text","text":"Explain instead."}]}}},
			{"dir":"sse","msg":{"type":"session_update","data":{"sessionUpdate":"agent_message_chunk"}}},
			{"dir":"sse","msg":{"type":"turn_complete","data":{"stopReason":"end_turn"}}}]}`)
	})
	sc.Step(`^the host starts Qwen Code at level 1 and declines the command$`, q.declineThenExplain)
	sc.Step(`^both Qwen Code turns end on session "([^"]+)" in 1 process$`, q.oneProcess)
	sc.Step(`^a scripted Qwen Code daemon without session "([^"]+)"$`, func(id string) error {
		return q.script(`{"session":"` + id + `","steps":[{"dir":"http","msg":{"call":"POST /session/` + id + `/resume","body":{},"status":404}}]}`)
	})
	sc.Step(`^the host resumes Qwen Code session "([^"]+)"$`, func(id string) error {
		return q.run(id, func(host.Process, host.Event) error { return nil })
	})
	sc.Step(`^the Qwen Code driver reports no session and refuses terminal takeover$`, q.noSession)
}

func (q *qwenAcceptance) script(text string) error {
	dir := q.f.t.TempDir()
	q.log = filepath.Join(dir, "log")
	for key, value := range map[string]string{"SOLODAWN_FAKE_QWEN_SCRIPT": filepath.Join(dir, "script.json"), "SOLODAWN_FAKECLI_LOG": q.log} {
		old, had := os.LookupEnv(key)
		os.Setenv(key, value)
		q.undo(func() { restoreVar(key, old, had) })
	}
	return os.WriteFile(filepath.Join(dir, "script.json"), []byte(text), 0600)
}

// run starts the driver and records its events until the daemon ends,
// calling act on each.
func (q *qwenAcceptance) run(session string, act func(host.Process, host.Event) error) error {
	l := host.Launch{Folder: q.f.t.TempDir(), Session: session, Level: 1, Shim: []string{"solodawn", "mcp"}, ToolTimeout: time.Minute}
	proc, err := host.Drivers["qwen"].Start(q.f.t.Context(), l)
	if err != nil {
		return err
	}
	defer proc.Close()
	timeout := time.After(30 * time.Second)
	for err == nil {
		select {
		case e, ok := <-proc.Events():
			if !ok {
				return nil
			}
			q.events = append(q.events, e)
			err = act(proc, e)
		case <-timeout:
			err = fmt.Errorf("qwen events so far: %+v", q.events)
		}
	}
	return err
}

func (q *qwenAcceptance) declineThenExplain() error {
	turns := []string{"Run the tests.", "Explain instead."}
	return q.run("", func(proc host.Process, e host.Event) error {
		switch {
		case e.Kind == host.Asked:
			return proc.Answer(e.Ask.ID, false)
		case e.Kind == host.Ready || e.Kind == host.TurnEnded && len(turns) > 0:
			turn := turns[0]
			turns = turns[1:]
			return proc.Turn(turn)
		}
		return nil
	})
}

func (q *qwenAcceptance) oneProcess(session string) error {
	var kinds []string
	for _, e := range q.events {
		kinds = append(kinds, string(e.Kind)+":"+e.Session+e.Status)
	}
	want := "ready:" + session + " asked: turn_ended:completed active: turn_ended:completed"
	log, err := os.ReadFile(q.log)
	if got := strings.Join(kinds, " "); got != want || err != nil || strings.Count(string(log), "start qwen") != 1 {
		return fmt.Errorf("events %q, want %q; log %q, %v", got, want, log, err)
	}
	return nil
}

func (q *qwenAcceptance) noSession() error {
	_, _, err := host.Drivers["qwen"].Interactive(host.Launch{})
	if len(q.events) != 1 || q.events[0].Kind != host.NoSession || err == nil {
		return fmt.Errorf("events %+v, takeover error %v", q.events, err)
	}
	return nil
}
