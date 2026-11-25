//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() { stepSets["home_rounds_held"] = heldMentionSteps }

// heldMentionWorld joins the two halves of A5.8: a real home that pauses the
// task, and a real host that holds the mention. turns has the items of each
// turn the host composed, in order.
type heldMentionWorld struct {
	*m4World
	task  string
	tenth string // the message id of the 10th mention

	mu    sync.Mutex
	turns []heldTurn
}

type heldTurn struct {
	session host.SessionKey
	msgs    []string
}

func heldMentionSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &heldMentionWorld{m4World: &m4World{f: f, seenCards: map[string]bool{}}}
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.activeHost != nil {
			waitHostDormant(w.activeHost)
		}
		return ctx, nil
	})
	w.cleanup = afterScenario(sc)
	w.tb = scenarioTB{TB: f.t, cleanup: w.cleanup}
	sc.Step(`^a project whose AIs Claude and Codex answer each other in the thread of Claude's task$`, w.setup)
	sc.Step(`^their 10th mention pauses the task with wait "rounds"$`, w.awaitPause)
	sc.Step(`^Claude answers a newer mention in the main line while the 10th mention stays on the home, in no turn$`, w.checkHeld)
	sc.Step(`^the owner ends the pause by (a message in the thread|task resume)$`, w.endPause)
	sc.Step(`^the host gives Claude's task session a turn with the 10th mention, and the home has no item left$`, w.checkReleased)
}

func (w *heldMentionWorld) setup() error {
	w.recordTurns()
	if err := w.givenTwoAIGroup(0, "Codex", "Claude"); err != nil {
		return err
	}
	if err := w.setScript("claude", map[string]any{"steps": heldClaudeTurns()}); err != nil {
		return err
	}
	if err := w.setScript("codex", map[string]any{"session": m4CodexThread, "steps": heldCodexTurns()}); err != nil {
		return err
	}
	var err error
	if w.task, err = w.startTaskFor("Claude", "Argue", "Answer each other"); err != nil {
		return err
	}
	// The next process of Claude is its group session.
	if _, err = w.waitLogs("start claude ", 1); err != nil {
		return err
	}
	return w.setScript("claude", map[string]any{"steps": claudeTurnSteps(1, true, "", "Main line answered.", "")})
}

// recordTurns notes the items of every turn before the host gives it.
func (w *heldMentionWorld) recordTurns() {
	compose := host.Compose
	host.Compose = func(h *host.Host, s host.Session, kind host.TurnKind, items []json.RawMessage) (string, host.Turn, error) {
		turn := heldTurn{session: s.SessionKey}
		for _, raw := range items {
			var wire proto.InboxItem
			if err := proto.Decode(raw, &wire); err == nil && wire.Item.Why == "mention" {
				turn.msgs = append(turn.msgs, string(wire.Item.Msg))
			}
		}
		w.mu.Lock()
		w.turns = append(w.turns, turn)
		w.mu.Unlock()
		return compose(h, s, kind, items)
	}
	w.cleanup(func() { host.Compose = compose })
}

// turnsWith lists the sessions whose turns had the mention of msg.
func (w *heldMentionWorld) turnsWith(msg string) []host.SessionKey {
	w.mu.Lock()
	defer w.mu.Unlock()
	var got []host.SessionKey
	for _, turn := range w.turns {
		if slices.Contains(turn.msgs, msg) {
			got = append(got, turn.session)
		}
	}
	return got
}

// heldClaudeTurns mentions Codex in each of five turns of the task session.
func heldClaudeTurns() []map[string]any {
	var steps []map[string]any
	for n := 1; n <= 5; n++ {
		steps = append(steps, claudeToolTurn(n == 1, "", "", heldMention(n, "Codex", 2*n-1))...)
	}
	return steps
}

// heldCodexTurns mentions Claude back in each of five turns.
func heldCodexTurns() []map[string]any {
	steps := codexStartupSteps()
	for n := 1; n <= 5; n++ {
		steps = append(steps, codexToolTurn(fmt.Sprintf("turn-%d", n), "", heldMention(n, "Claude", 2*n))...)
	}
	return steps
}

func heldMention(callID int, to string, round int) []map[string]any {
	args := map[string]any{"text": fmt.Sprintf("@%s round %d", to, round), "mentions": []string{to}}
	return mcpCallSteps(callID, "post", args, map[string]any{"isError": false})
}

func (w *heldMentionWorld) awaitPause() error {
	paused := func() bool {
		t := w.b1.tasks[w.task]
		return t.State == "paused" && t.Wait != nil && t.Wait.Kind == "rounds" && t.Thread.Rounds == 10
	}
	if err := w.b1.drainUntil(20*time.Second, paused, nil); err != nil {
		return fmt.Errorf("task %+v: %w", w.b1.tasks[w.task], err)
	}
	for id, msg := range w.b1.msgs {
		if msg.Text == "@Claude round 10" {
			w.tenth = id
		}
	}
	if _, limit := m4SystemMsg(w.b1, "rounds_limit"); w.tenth == "" || !limit {
		return fmt.Errorf("10th mention %q, rounds_limit message %v", w.tenth, limit)
	}
	return nil
}

// checkHeld has the owner's mention pass the held one: the host takes an AI's
// items in order, so one it did not hold would have gone first.
func (w *heldMentionWorld) checkHeld() error {
	if err := w.postOwnerMention("Claude", "@Claude and in the main line?"); err != nil {
		return err
	}
	// Settled: only the end of the pause can give the next turn.
	if err := w.thenAIPostsAndSettles("Claude", "Main line answered."); err != nil {
		return err
	}
	if err := awaitHomeInboxCount(w.node().Store, 1); err != nil {
		return err
	}
	if turns := w.turnsWith(w.tenth); len(turns) != 0 {
		return fmt.Errorf("the 10th mention went to %v while the task was paused", turns)
	}
	return nil
}

func (w *heldMentionWorld) endPause(how string) error {
	if err := w.setScript("claude", map[string]any{"steps": claudeTurnSteps(1, true, "", "Held mention answered.", "")}); err != nil {
		return err
	}
	head := proto.HomeCommand{Type: "task", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), Group: w.groupID}
	if how == "task resume" {
		return w.sendOK(w.b1, proto.TaskCommand{HomeCommand: head, Op: "resume", ID: proto.ID(w.task)})
	}
	head.Type = "post"
	thread := proto.ID(w.task)
	return w.sendOK(w.b1, proto.Post[proto.EmptyBody]{HomeCommand: head, Kind: "text", Text: "Keep the direct path simple.", Thread: &thread, Mentions: []proto.ID{}})
}

func (w *heldMentionWorld) checkReleased() error {
	if err := w.awaitAIPost("Claude", "Held mention answered.", 10*time.Second); err != nil {
		return err
	}
	session := host.SessionKey{Group: string(w.groupID), Member: w.b1.aiByName["Claude"].ID, Task: w.task}
	if turns := w.turnsWith(w.tenth); !slices.Equal(turns, []host.SessionKey{session}) {
		return fmt.Errorf("the 10th mention went to %v, want one turn of %v", turns, session)
	}
	if t := w.b1.tasks[w.task]; t.State != "active" || t.Wait != nil || t.Thread.Rounds != 0 {
		return fmt.Errorf("task after the pause: %+v", t)
	}
	return awaitHomeInboxCount(w.node().Store, 0)
}
