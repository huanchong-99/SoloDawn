//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["host_turn_failed"] = hostTurnFailedSteps
}

type loginChangedSnap struct {
	sessionState host.State
	open         int
	memberState  string
	loginOK      bool
}

type loginSnapRecorder struct {
	mu    sync.Mutex
	snaps []loginChangedSnap
}

// record runs on the host's goroutines, so it is given the ids and reads
// nothing of the scenario's view, which the steps write.
func (r *loginSnapRecorder) record(h *host.Host, gid, aiID, cli string) {
	s, _ := h.Sessions.Get(host.SessionKey{Group: gid, Member: aiID})
	open := 0
	for _, sess := range h.Sessions.All() {
		if sess.State != host.Dormant {
			open++
		}
	}
	memState, _ := h.MemberState(gid, aiID)
	agents, err := h.Agents(context.Background(), false)
	loginOK := false
	if err == nil {
		for _, a := range agents {
			if a.Agent == cli {
				loginOK = a.LoginOK
			}
		}
	}
	r.mu.Lock()
	r.snaps = append(r.snaps, loginChangedSnap{
		sessionState: s.State,
		open:         open,
		memberState:  memState,
		loginOK:      loginOK,
	})
	r.mu.Unlock()
}

func (r *loginSnapRecorder) snapshot() []loginChangedSnap {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]loginChangedSnap(nil), r.snaps...)
}

func hostTurnFailedSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &m3World{
		f:        f,
		groups:   map[string]string{},
		homeView: newHomeView(),
	}
	rec := &loginSnapRecorder{}
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.activeHost != nil {
			waitHostDormant(w.activeHost)
		}
		return ctx, nil
	})
	w.cleanup = afterScenario(sc)
	w.tb = scenarioTB{TB: f.t, cleanup: w.cleanup}
	noConfirmCardAfter(sc, func() map[string]proto.MsgRow { return w.msgs })

	sc.Step(`^a group "([^"]+)" with a configured "([^"]+)" member on "([^"]+)" whose failed turn keeps the CLI open$`, w.givenSingleAINode)
	sc.Step(`^the host records LoginChanged snapshots for "([^"]+)" in "([^"]+)"$`, func(aiName, groupName string) {
		h, gid, ai := w.node().Host, w.groups[groupName], w.aiByName[aiName]
		aiID, cli := ai.ID, ai.CLI
		h.LoginChanged = func() { rec.record(h, gid, aiID, cli) }
	})
	sc.Step(`^"([^"]+)" is scripted to fail once with "([^"]+)" and later post "([^"]+)"$`, func(_, detail, reply string) error {
		return scriptClaudeFailedThenReply(w, detail, reply)
	})
	sc.Step(`^"([^"]+)" is scripted to fail sign-in with "([^"]+)"$`, func(_, detail string) error {
		return scriptClaudeLoginInvalid(w, detail)
	})
	sc.Step(`^the owner sends "([^"]+)" to "([^"]+)"$`, w.whenOwnerPosts)
	sc.Step(`^a "turn_interrupted" crash row for "([^"]+)" with detail "([^"]+)" is posted in "([^"]+)"$`, func(aiName, detail, _ string) error {
		return awaitInterruptedDetail(w, aiName, "crash", detail)
	})
	sc.Step(`^a "login_invalid" notice for "([^"]+)" with detail "([^"]+)" is posted in "([^"]+)"$`, func(aiName, detail, _ string) error {
		return awaitLoginInvalidNotice(w, aiName, detail)
	})
	sc.Step(`^"([^"]+)" (?:stays in|enters) state "([^"]+)" in "([^"]+)"$`, w.thenAIHasState)
	sc.Step(`^LoginChanged fired (once|twice) with session "([^"]+)", (\d+) open sessions, member state "([^"]+)", and "([^"]+)" login_ok (true|false)$`, func(times, sessState string, wantOpen int, memState, _ string, wantLogin string) error {
		return awaitLoginChangedSnapshot(rec, map[string]int{"once": 1, "twice": 2}[times], host.State(sessState), wantOpen, memState, wantLogin == "true")
	})
	sc.Step(`^owner device "([^"]+)" sends agent\.recheck for "([^"]*)"$`, func(by, agent string) error {
		payload := fmt.Sprintf(`{"t":"agent.recheck","n":"STwCAvPRTFkRNCvXYEgsMw","ts":1763600000000,"to":"host-device","agent":%q}`, agent)
		_, err := host.Surface["agent.recheck"](context.Background(), w.node().Host, nil, by, json.RawMessage(payload))
		return err
	})
	sc.Step(`^"([^"]+)" replies with "([^"]+)" in "([^"]+)"$`, w.thenAIPosts)
	sc.Step(`^the fake "([^"]+)" log records 1 process for that group conversation$`, w.thenOneProcessLogged)
}

func scriptClaudeLoginInvalid(w *m3World, detail string) error {
	steps := []map[string]any{claudeUserStdinStep("")}
	steps = append(steps, claudeInitMCP(0)...)
	steps = append(steps,
		map[string]any{"dir": "stdout", "msg": map[string]any{"type": "system", "subtype": "init", "session_id": "$session"}},
		map[string]any{"dir": "stdout", "msg": map[string]any{
			"type":       "result",
			"subtype":    "success",
			"is_error":   true,
			"result":     detail,
			"session_id": "$session",
		}},
	)
	return w.setScript("claude", map[string]any{"steps": steps, "exit": 1})
}

func awaitLoginInvalidNotice(w *m3World, aiName, wantDetail string) error {
	aiID := w.aiByName[aiName].ID
	found := func() bool {
		return viewHasLoginInvalidDetail(&w.homeView, aiID, wantDetail)
	}
	return w.drainBrowserUntil(10*time.Second, found, nil)
}

func viewHasLoginInvalidDetail(v *homeView, aiID, wantDetail string) bool {
	for _, m := range v.msgs {
		if isLoginInvalidMsg(m, aiID, wantDetail) {
			return true
		}
	}
	return false
}

func isLoginInvalidMsg(m proto.MsgRow, aiID, wantDetail string) bool {
	if m.Kind != "system" {
		return false
	}
	var body proto.LoginInvalidBody
	if proto.Decode(m.Body, &body) != nil || body.Detail == nil {
		return false
	}
	return body.Code == "login_invalid" && body.AI == aiID && *body.Detail == wantDetail
}

// awaitLoginChangedSnapshot waits for the n-th snapshot, which is the last and
// equals what is wanted.
func awaitLoginChangedSnapshot(rec *loginSnapRecorder, n int, wantSess host.State, wantOpen int, wantMem string, wantLogin bool) error {
	want := loginChangedSnap{sessionState: wantSess, open: wantOpen, memberState: wantMem, loginOK: wantLogin}
	return waitUntil(5*time.Second, "LoginChanged snapshot", func() error {
		snaps := rec.snapshot()
		if len(snaps) < n {
			return fmt.Errorf("%d LoginChanged snapshots yet, want %d", len(snaps), n)
		}
		if len(snaps) != n || snaps[n-1] != want {
			return abort(fmt.Errorf("LoginChanged snapshots = %+v, want %d, the last %+v", snaps, n, want))
		}
		return nil
	})
}

func scriptClaudeFailedThenReply(w *m3World, detail, reply string) error {
	unescaped := strings.ReplaceAll(detail, `\n`, "\n")
	steps := []map[string]any{claudeUserStdinStep("")}
	steps = append(steps, claudeInitMCP(0)...)
	steps = append(steps,
		map[string]any{"dir": "stdout", "msg": map[string]any{"type": "system", "subtype": "init", "session_id": "$session"}},
		map[string]any{"dir": "stdout", "msg": map[string]any{
			"type":             "result",
			"subtype":          "error_during_execution",
			"is_error":         true,
			"api_error_status": 503,
			"result":           unescaped,
			"session_id":       "$session",
		}},
	)
	steps = append(steps, claudeTurnSteps(1, false, "", reply, "")...)
	return w.setScript("claude", map[string]any{"steps": steps})
}

func awaitInterruptedDetail(w *m3World, aiName, wantReason, wantDetail string) error {
	aiID := w.aiByName[aiName].ID
	found := func() bool {
		return viewHasInterruptedDetail(&w.homeView, aiID, wantReason, wantDetail)
	}
	return w.drainBrowserUntil(10*time.Second, found, nil)
}

func viewHasInterruptedDetail(v *homeView, aiID, wantReason, wantDetail string) bool {
	for _, m := range v.msgs {
		if isInterruptedMsgWithDetail(m, aiID, wantReason, wantDetail) {
			return true
		}
	}
	return false
}

func isInterruptedMsgWithDetail(m proto.MsgRow, aiID, wantReason, wantDetail string) bool {
	if !isInterruptedMsg(m, aiID, wantReason) {
		return false
	}
	var body proto.TurnInterruptedBody
	if proto.Decode(m.Body, &body) != nil || body.Detail == nil {
		return false
	}
	return *body.Detail == wantDetail
}
