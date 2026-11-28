//go:build acceptance

package features

import (
	"context"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["host_turn_failed"] = hostTurnFailedSteps
}

func hostTurnFailedSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &m3World{
		f:        f,
		groups:   map[string]string{},
		homeView: newHomeView(),
	}
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
	sc.Step(`^"([^"]+)" is scripted to fail once with "([^"]+)" and later post "([^"]+)"$`, func(_, detail, reply string) error {
		return scriptClaudeFailedThenReply(w, detail, reply)
	})
	sc.Step(`^the owner sends "([^"]+)" to "([^"]+)"$`, w.whenOwnerPosts)
	sc.Step(`^a "turn_interrupted" crash row for "([^"]+)" with detail "([^"]+)" is posted in "([^"]+)"$`, func(aiName, detail, _ string) error {
		return awaitInterruptedDetail(w, aiName, "crash", detail)
	})
	sc.Step(`^"([^"]+)" stays in state "([^"]+)" in "([^"]+)"$`, w.thenAIHasState)
	sc.Step(`^"([^"]+)" replies with "([^"]+)" in "([^"]+)"$`, w.thenAIPosts)
	sc.Step(`^the fake "([^"]+)" log records 1 process for that group conversation$`, w.thenOneProcessLogged)
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
