package host

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// pushRole pushes the row of the AI as the home does when the group's secretary
// or the AI's name is changed, and waits until the host has acted on it.
func (r *jumpRig) pushRole(name string, secretary bool, seq int64) {
	row := proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: runAI1, Seq: seq}, Group: runGroup1, Kind: "ai", Name: name},
		CLI:       "claude", Host: r.h.Key.ID(), Secretary: secretary, Allow: []string{}, Approvers: []string{},
	}
	frame := proto.Rows{Type: "rows", Group: runGroup1, Rows: []json.RawMessage{json.RawMessage(marshalWire(row))}}
	r.h.Push(jumpHome, json.RawMessage(marshalWire(frame)))
	synctest.Wait()
}

// wantRole checks the role of the CLI that has the group session: its
// instructions, its tools, its role and its name.
func (r *jumpRig) wantRole(p *fakeProc, name string, secretary bool, session string) {
	r.t.Helper()
	c, _ := r.h.Sessions.Caller(tokenFromLaunch(p.launch))
	_, brief := ToolsFor(c)["propose_brief"]
	_, start := ToolsFor(c)["start_task"]
	joinAssertEqual(r.t, "propose_brief", brief, secretary)
	joinAssertEqual(r.t, "start_task", start, !secretary)
	joinAssertEqual(r.t, "name", c.Name, name)
	joinAssertEqual(r.t, "secretary's instructions", p.launch.Instructions == secretaryInstructions, secretary)
	joinAssertEqual(r.t, "resumed session", p.launch.Session, session)
	text := jumpTurns(p)[0]
	joinAssertEqual(r.t, "named in "+text, strings.Contains(text, "you are \""+name+"\""), true)
	joinAssertEqual(r.t, "secretary in "+text, strings.Contains(text, "secretary session"), secretary)
}

func roleClosed(p *fakeProc) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closedIn
}

func roleTest(t *testing.T, run func(t *testing.T, r *jumpRig)) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		Instructions, Compose = instructionsFor, composeTurn
		run(t, r)
	})
}

func roleStart(r *jumpRig, item int64) *fakeProc {
	r.push(jumpMention(item, testMsg1, "", false))
	p := r.drv.lastProc()
	p.emit(Event{Kind: Ready, Session: "cli-1"})
	synctest.Wait()
	return p
}

// The rows say which AI is the secretary and how an AI is named; its group
// session follows at its next turn, in a new CLI session when the role
// changed, and a turn that is running goes on.
func TestGroupSessionFollowsTheSecretaryOfTheRows(t *testing.T) {
	roleTest(t, func(t *testing.T, r *jumpRig) {
		worker := roleStart(r, 1)
		r.wantRole(worker, "Claude Code", false, "")
		r.pushRole("Lead", true, 3)
		if worker.interrupts.Load() != 0 || roleClosed(worker) {
			t.Fatal("the change cut the running turn")
		}
		r.wantRole(worker, "Claude Code", false, "")
		r.end(worker, "completed")

		lead := roleStart(r, 2)
		if !roleClosed(worker) {
			t.Fatal("the CLI of the old role still runs")
		}
		r.wantRole(lead, "Lead", true, "")
		r.end(lead, "completed")

		r.pushRole("Renamed", true, 4)
		r.push(jumpMention(3, testMsg1, "", false))
		if turns := jumpTurns(lead); len(turns) != 2 || !strings.Contains(turns[1], `you are "Renamed" · secretary session`) {
			t.Fatalf("the renamed AI's next turn: %q", turns)
		}
		r.end(lead, "completed")

		r.pushRole("Renamed", false, 5)
		r.wantRole(roleStart(r, 4), "Renamed", false, "")
	})
}

// The rows changed while the host was down: the saved session follows them.
func TestSavedSessionFollowsTheSecretaryOfTheRows(t *testing.T) {
	saveRuntimeGlobals(t)
	dir := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		start := func() *jumpRig {
			r := newJumpRig(t, dir)
			Instructions, Compose = instructionsFor, composeTurn
			return r
		}
		r := start()
		r.end(roleStart(r, 1), "completed")
		r.close()

		r = start()
		defer r.close()
		r.pushRole("Lead", true, 3)
		r.wantRole(roleStart(r, 2), "Lead", true, "")
	})
}

// A session that cannot be changed gets no turn, and its items wait.
func TestSessionThatCannotFollowItsRowHoldsTheItems(t *testing.T) {
	roleTest(t, func(t *testing.T, r *jumpRig) {
		r.end(roleStart(r, 1), "completed")
		r.pushRole("Lead", false, 3)
		block := func(stmt string) {
			r.db.Write(func(tx *store.Tx) error {
				_, err := tx.Exec(stmt)
				return err
			})
		}
		block("CREATE TRIGGER block BEFORE INSERT ON session BEGIN SELECT RAISE(ABORT, 'blocked'); END")
		r.push(jumpMention(2, testMsg1, "", false))
		r.wantHeld(2)
		block("DROP TRIGGER block")
		r.deliver()
		r.wantAcked(2)
	})
}
