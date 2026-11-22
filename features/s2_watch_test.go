//go:build acceptance

package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() { stepSets["s2_watch"] = s2WatchSteps }

type s2WatchWorld struct {
	f                 fixtures
	cleanup           func(func())
	c                 *s2Cast
	team, project     proto.ID
	secretary, worker proto.AIMemberRow
	teamOwner         string
}

func s2WatchSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &s2WatchWorld{f: f, cleanup: afterScenario(sc)}
	sc.Step(`^P creates a team group on P2 without a repository$`, w.createTeam)
	sc.Step(`^its group row has no repository$`, w.teamRow)
	sc.Step(`^the team group refuses task start, brief posts, host\.join, member add_ai and check settings$`, w.teamRefusals)
	sc.Step(`^no node holds a clone for the team group$`, w.noTeamClone)
	sc.Step(`^the secretary's node knows the team group$`, w.prepareBulletin)
	sc.Step(`^the secretary posts a notice to the team group using the post tool$`, w.postBulletin)
	sc.Step(`^the team main line shows a bulletin naming the secretary and project$`, w.checkBulletin)
	sc.Step(`^the working AI's call to the same tool is refused$`, w.refuseBulletin)
	sc.Step(`^P posts a team message naming the working AI$`, w.postTeamText)
	sc.Step(`^the team message creates no inbox item$`, w.noTeamInbox)
	s2RegisterTerminal(sc, w)
	s2RegisterForge(sc, w)
}

func (w *s2WatchWorld) start(timeout time.Duration) error {
	c, err := startS2Cast(w.f.t, w.cleanup, w.f, s2CastConfig{Timeout: timeout})
	w.c = c
	return err
}

func s2WatchCmd(c *s2Cast, p *s2Person, conn *peer.Connection, fields map[string]any, want string) error {
	fields["ts"] = time.Now().UnixMilli()
	if _, ok := fields["n"]; !ok {
		fields["n"] = c.NextID()
	}
	if want == "" {
		_, err := c.SendCmd(p, conn, fields)
		return err
	}
	raw, err := sendSignedCmdRaw(c.Ctx, conn, p.Key, fields)
	if err != nil {
		return err
	}
	return s2WatchReply(raw, want)
}

func s2WatchReply(raw json.RawMessage, want string) error {
	if want == "" {
		if err := proto.Decode(raw, new(proto.Success)); err != nil {
			return fmt.Errorf("expected a successful reply, got %s: %w", raw, err)
		}
		return nil
	}
	return checkWireError(raw, want)
}

func (w *s2WatchWorld) createTeam() error {
	if err := w.start(3 * time.Minute); err != nil {
		return err
	}
	c := w.c
	w.team = c.NextID()
	fields := map[string]any{"t": "group", "op": "create", "g": w.team, "n": w.team,
		"name": "Team", "member_name": c.P.Name, "kind": "team", "base": "", "forge": "none", "checks": []string{}}
	conn := c.P.Conns[c.P2.ID()].conn
	if err := s2WatchCmd(c, c.P, conn, fields, ""); err != nil {
		return err
	}
	return c.syncGroup(conn, w.team)
}

func (w *s2WatchWorld) teamRow() error {
	c := w.c
	return c.readGroupRowsUntil(c.P.Conns[c.P2.ID()], w.team, checkS2TeamRow)
}

func checkS2TeamRow(raw json.RawMessage) (bool, error) {
	var row proto.GroupRow
	if err := proto.Decode(raw, &row); err != nil || row.Type != "group" {
		return false, nil
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	if row.Kind != "team" || fields["repo"] != nil {
		return false, fmt.Errorf("unexpected team group row: %s", raw)
	}
	return true, checkS2TeamDefaults(row)
}

func checkS2TeamDefaults(row proto.GroupRow) error {
	if row.Base != "" || row.Forge != "none" || len(row.Checks) != 0 {
		return fmt.Errorf("unexpected team group defaults: %+v", row)
	}
	return nil
}

func (w *s2WatchWorld) teamRefusals() error {
	c := w.c
	home := c.P.Conns[c.P2.ID()].conn
	fields := []map[string]any{
		{"t": "task", "op": "start", "id": c.NextID(), "title": "Build", "plan": "Build it", "re": nil, "origin": "direct"},
		s2WatchPost(w.team, "brief", "", proto.BriefBody{
			Title: "Build", Goal: "Build it", Criteria: []string{"Build passes"},
			Scope: proto.BriefScope{In: []string{}, Out: []string{}}, Tasks: []proto.BriefTask{}, Pass: 90,
		}),
		{"t": "member", "op": "add_ai", "id": c.NextID(), "name": "AI", "cli": s2SecretaryCLI,
			"account": "team", "secretary": false, "approvers": []string{}},
		{"t": "group", "op": "set", "checks": []string{"git status"}},
	}
	for _, cmd := range fields {
		cmd["g"] = w.team
		want := "conflict"
		if cmd["t"] == "group" {
			want = "bad_request"
		}
		if err := s2WatchCmd(c, c.P, home, cmd, want); err != nil {
			return fmt.Errorf("%s %v: %w", cmd["t"], cmd["op"], err)
		}
	}
	join := map[string]any{"t": "host.join", "to": c.P1.ID(), "g": w.team, "home": c.P2.ID(), "agent": s2SecretaryCLI, "name": "AI", "secretary": false}
	return s2WatchCmd(c, c.P, c.P.Conns[c.P1.ID()].conn, join, "conflict")
}

func (w *s2WatchWorld) noTeamClone() error {
	for _, n := range []*s2Node{w.c.P1, w.c.P2, w.c.Q1} {
		path := filepath.Join(n.Data, "projects", string(w.team))
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s team project directory: stat error %v, want absent", n.Name, err)
		}
	}
	return nil
}

func (w *s2WatchWorld) prepareBulletin() error {
	c := w.c
	var err error
	w.project, err = c.CreateProjectGroup(c.P, c.P1, "Website", "none")
	if err != nil {
		return err
	}
	w.secretary, err = c.JoinAI(c.P, c.P1, c.P1, w.project, s2SecretaryCLI, s2SecretaryName, true)
	if err != nil {
		return err
	}
	w.worker, err = c.JoinAI(c.P, c.P2, c.P1, w.project, s2PWorkCLI, s2PWorkName, false)
	if err != nil {
		return err
	}
	fields := map[string]any{"t": "home.add", "g": w.team, "home": c.P2.ID(), "to": c.P1.ID(), "name": "Team", "addrs": []string{c.RVURL}}
	if err := s2WatchCmd(c, c.P, c.P.Conns[c.P1.ID()].conn, fields, ""); err != nil {
		return err
	}
	return c.readGroupRowsUntil(c.P.Conns[c.P2.ID()], w.team, func(raw json.RawMessage) (bool, error) {
		var row proto.HumanMemberRow
		if proto.Decode(raw, &row) != nil || row.Role != "owner" {
			return false, nil
		}
		w.teamOwner = row.ID
		return true, nil
	})
}

const s2BulletinText = "Release candidate is ready"

func (w *s2WatchWorld) postBulletin() error {
	c := w.c
	post := m5ToolCallSpec{Name: "post", Args: map[string]any{"group": w.team, "text": s2BulletinText}}
	if err := c.WriteScript(c.P1, s2SecretaryCLI, m5ClaudeScript([]m5ToolCallSpec{post})); err != nil {
		return err
	}
	return postS2Mention(c, c.P, c.P1, w.project, w.secretary.ID, "Post a team notice")
}

func (w *s2WatchWorld) checkBulletin() error {
	c := w.c
	return c.readGroupRowsUntil(c.P.Conns[c.P2.ID()], w.team, func(raw json.RawMessage) (bool, error) {
		var row proto.MsgRow
		if proto.Decode(raw, &row) != nil || row.Kind != "bulletin" || row.Text != s2BulletinText {
			return false, nil
		}
		return true, w.bulletinRow(row)
	})
}

func (w *s2WatchWorld) bulletinRow(row proto.MsgRow) error {
	if err := w.bulletinPlacement(row); err != nil {
		return err
	}
	var body struct {
		From  proto.ID `json:"from"`
		Group string   `json:"group"`
		AI    string   `json:"ai"`
	}
	if err := proto.Decode(row.Body, &body); err != nil {
		return err
	}
	if body.From != w.project || body.Group != "Website" || body.AI != s2SecretaryName {
		return fmt.Errorf("unexpected bulletin attribution: %+v", body)
	}
	return nil
}

func (w *s2WatchWorld) bulletinPlacement(row proto.MsgRow) error {
	if row.By != w.teamOwner || row.State != nil {
		return fmt.Errorf("unexpected bulletin author or kind: %+v", row)
	}
	if row.Thread != nil || row.Main || row.Re != nil {
		return fmt.Errorf("bulletin is outside the team main line: %+v", row)
	}
	return nil
}

func (w *s2WatchWorld) refuseBulletin() error {
	c := w.c
	const done = "Worker bulletin call was refused"
	calls := []m5ToolCallSpec{
		{Name: "post", Args: map[string]any{"group": w.team, "text": s2BulletinText}, WantErr: true},
		{Name: "post", Args: map[string]any{"text": done}},
	}
	if err := c.WriteScript(c.P2, s2PWorkCLI, m5ClaudeScript(calls)); err != nil {
		return err
	}
	if err := postS2Mention(c, c.P, c.P1, w.project, w.worker.ID, "Try to post the team notice"); err != nil {
		return err
	}
	var saw bool
	return c.readGroupRowsUntil(c.P.Conns[c.P1.ID()], w.project, func(raw json.RawMessage) (bool, error) {
		return matchS2PostThenWaiting(raw, w.worker.ID, done, &saw), nil
	})
}

func (w *s2WatchWorld) postTeamText() error {
	c := w.c
	fields := s2WatchPost(w.team, "text", "Ask @"+s2PWorkName+" about the release", proto.EmptyBody{})
	return s2WatchCmd(c, c.P, c.P.Conns[c.P2.ID()].conn, fields, "")
}

func (w *s2WatchWorld) noTeamInbox() error {
	db, err := store.Open(filepath.Join(w.c.P2.Data, "node.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Write(func(tx *store.Tx) error {
		var n int
		if err := tx.QueryRow("SELECT count(*) FROM inbox WHERE g = ?", w.team).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			return fmt.Errorf("team group has %d inbox items", n)
		}
		return nil
	})
}
