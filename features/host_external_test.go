//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["host_external"] = hostExternalSteps
}

type extScenario struct {
	join     joinScenarioWorld
	refusals []error
	first    proto.Handout
	runs     []proto.Handout
	states   []string
	none     bool
	cmds     []map[string]any
}

func hostExternalSteps(sc *godog.ScenarioContext, f fixtures) {
	cleanup := afterScenario(sc)
	s := &extScenario{}
	sc.Step(`^a host on disk whose remote home "([^"]+)" hosts the project group "([^"]+)"$`, func(remote, group string) error {
		if err := s.join.initHostOnDisk("claude", "codex", remote, cleanup); err != nil {
			return err
		}
		return s.join.createRemoteGroup(remote, group)
	})
	sc.Step(`^the owner joins the external AI "([^"]+)" with account "([^"]+)"(?: and tool "([^"]+)")? to "([^"]+)"$`, func(name, account, tool, group string) error {
		return s.joinExternal(proto.HostJoin{Name: name, Account: account, Tool: optionalTool(tool)}, group)
	})
	sc.Step(`^joining an external AI without an account, with account "free" or as secretary is "bad_request"$`, s.checkJoinRefusals)
	sc.Step(`^the home added 2 members with cli "external" and the host holds 2 external group sessions with no CLI session, both "offline"$`, s.checkJoined)
	sc.Step(`^the host's agents end with the agent "external" with takeover "none" and no models$`, s.checkAgents)
	sc.Step(`^a host whose external AI "Scout" is present and whose group has turn_minutes 1$`, func() { *s = extScenario{} })
	sc.Step(`^the home mentions "Scout" twice and the runner takes, ends, fails and abandons runs$`, func() error {
		return runExtBubble(f.t, s.runEnds)
	})
	sc.Step(`^the first hand-out is run 1 of kind "group" in the clone, with the group and external instructions in its context and the items in its text$`, s.checkFirst)
	sc.Step(`^while run 1 was open nothing more was handed out and "Scout" was "working"$`, func() error {
		return s.checkStates(0, "working")
	})
	sc.Step(`^after run 1 ended "Scout" was "waiting", run 2 was handed out once and a late done for run 1 changed nothing$`, s.checkSecond)
	sc.Step(`^the failed run 2 was reported as turn_interrupted "crash" with "exit 1"$`, func() error {
		return s.checkInterrupted("crash", "exit 1")
	})
	sc.Step(`^the run nobody ended was reported as turn_interrupted "timeout" after 1 minute and "Scout" was "waiting"$`, func() error {
		if err := s.checkInterrupted("timeout", ""); err != nil {
			return err
		}
		return s.checkStates(2, "waiting")
	})
	sc.Step(`^the group is paused, all AIs go offline and the runner stops, each with an item waiting$`, func() error {
		return runExtBubble(f.t, s.runHeld)
	})
	sc.Step(`^"Scout" was "paused", "offline" and "offline" in turn, and no item was handed out$`, func() error {
		if s.none {
			return errors.New("an item was handed out while held")
		}
		return s.checkStates(0, "paused", "offline", "offline")
	})
	sc.Step(`^after the resume, the return online and the runner's return each waiting item was handed out$`, func() error {
		got := fmt.Sprint(runNumbers(s.runs))
		if got != "[1 2 3]" {
			return fmt.Errorf("handed out runs %s, want [1 2 3]", got)
		}
		return nil
	})
}

func optionalTool(tool string) *string {
	if tool == "" {
		return nil
	}
	return &tool
}

func (s *extScenario) joinExternal(p proto.HostJoin, group string) error {
	w := &s.join
	p.HostCommand = proto.HostCommand{Type: "host.join", Nonce: w.nextNonce(), Timestamp: time.Now().UnixMilli(), To: w.h.Key.ID()}
	p.Group, p.Home, p.Agent = joinGroupIDs[group], w.remoteDevice, "external"
	_, err := host.Surface["host.join"](context.Background(), w.h, nil, "owner-browser", json.RawMessage(accJSON(p)))
	return err
}

func (s *extScenario) checkJoinRefusals() error {
	for _, p := range []proto.HostJoin{{Name: "X"}, {Name: "X", Account: "free"}, {Name: "X", Account: "team", Secretary: true}} {
		var e *host.Error
		if err := s.joinExternal(p, "Website"); !errors.As(err, &e) || e.Code != "bad_request" {
			return fmt.Errorf("join %+v: %v, want bad_request", p, err)
		}
	}
	return nil
}

func (s *extScenario) checkJoined() error {
	w := &s.join
	w.remoteHome.mu.Lock()
	clis := fmt.Sprint(slices.Sorted(maps.Values(w.remoteHome.memberAgents)))
	w.remoteHome.mu.Unlock()
	if clis != "[external external]" {
		return fmt.Errorf("added clis %s", clis)
	}
	var got []string
	for _, sess := range w.h.Sessions.All() {
		state, _ := w.h.MemberState(sess.Group, sess.Member)
		got = append(got, fmt.Sprintf("%s %s %s %q %s", sess.Name, sess.Agent, sess.Kind, sess.CLISession, state))
	}
	want := `["Yan external group \"\" offline" "Zed external group \"\" offline"]`
	slices.Sort(got)
	if fmt.Sprintf("%q", got) != want {
		return fmt.Errorf("sessions %v, want %s", got, want)
	}
	return nil
}

func (s *extScenario) checkAgents() error {
	agents, err := s.join.h.Agents(context.Background(), false)
	if err != nil {
		return err
	}
	last := agents[len(agents)-1]
	if last.Agent != "external" || last.Driver.Takeover != "none" || len(last.Models) != 0 || !last.LoginOK {
		return fmt.Errorf("last agent %+v", last)
	}
	return nil
}

type extBubble struct {
	t      *testing.T
	h      *host.Host
	home   *hlHome
	homeID string
	base   string
	leave  func()
}

// runExtBubble runs a scenario on a host with turn_minutes 1 whose first AI
// is external and present, in one bubble, so that synctest.Wait orders it.
func runExtBubble(suiteT *testing.T, run func(*extBubble) error) error {
	var stepErr error
	base := suiteT.TempDir()
	prevDrivers, prevCompose, prevInstr := maps.Clone(host.Drivers), host.Compose, host.Instructions
	defer func() { host.Drivers, host.Compose, host.Instructions = prevDrivers, prevCompose, prevInstr }()
	synctest.Test(suiteT, func(t *testing.T) {
		installHLHooks(&hlDriver{})
		host.Instructions = func(s host.Session) string { return "instructions of " + string(s.Kind) }
		db, h, hostKey, homeKey, err := openHLHost(base, time.Now, 1)
		if err != nil {
			stepErr = err
			return
		}
		home := newHLHome(3)
		defer func() {
			home.stop()
			h.Settle()
			synctest.Wait()
			db.Close()
		}()
		h.Rows.Put(hlGroup, "member", hlAI1, json.RawMessage(accJSON(proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: hlAI1, Seq: 2}, Group: hlGroup, Kind: "ai", User: "u", Name: "Scout"},
			CLI:       "external", Host: hostKey.ID(), HostName: "host", Account: "team", Allow: []string{}, Approvers: []string{}, State: "offline", Path: "local",
		})))
		h.Sessions.Create(host.Session{SessionKey: host.SessionKey{Group: hlGroup, Member: hlAI1}, Kind: host.GroupSession, Folder: "repo", Agent: "external", Home: homeKey.ID(), Name: "Scout"})
		h.Attach(homeKey.ID(), home)
		synctest.Wait()
		leave := h.ExternalPresent(hlGroup, hlAI1)
		defer leave()
		b := &extBubble{t: t, h: h, home: home, homeID: homeKey.ID(), base: base, leave: leave}
		stepErr = run(b)
	})
	return stepErr
}

func (b *extBubble) mention(item int64) {
	pushHLMention(b.h, b.homeID, hlAI1, item)
	synctest.Wait()
}

func (b *extBubble) take() (proto.Handout, bool) {
	out, ok, err := b.h.ExternalTake(hlGroup, hlAI1)
	if err != nil {
		panic(err)
	}
	synctest.Wait()
	return out, ok
}

func (b *extBubble) state() string {
	state, _ := b.h.MemberState(hlGroup, hlAI1)
	return state
}

func (b *extBubble) done(run int64, failed string) {
	b.h.ExternalDone(hlGroup, hlAI1, run, failed)
	synctest.Wait()
}

func (s *extScenario) runEnds(b *extBubble) error {
	b.mention(1)
	s.first, _ = b.take()
	b.mention(2)
	_, more := b.take()
	s.none = more
	s.states = append(s.states, b.state())
	b.done(s.first.Run, "")
	s.states = append(s.states, b.state())
	second, _ := b.take()
	b.done(s.first.Run, "")
	_, third := b.take()
	s.runs = []proto.Handout{second}
	s.none = s.none || third
	b.done(second.Run, "exit 1")
	b.mention(3)
	b.take()
	time.Sleep(time.Minute)
	synctest.Wait()
	s.states = append(s.states, b.state())
	s.cmds = b.home.snapshotCmds()
	base, _ := filepath.EvalSymlinks(b.base)
	s.first.Folder = strings.TrimPrefix(s.first.Folder, filepath.Join(base, "projects", hlGroup))
	return nil
}

func (s *extScenario) runHeld(b *extBubble) error {
	grp, _ := b.h.Rows.Get(hlGroup, "group", hlGroup)
	var row proto.GroupRow
	if err := proto.Decode(grp, &row); err != nil {
		return err
	}
	paused := row
	paused.Paused, paused.Seq = &proto.GroupPause{By: "u", At: 1}, 4
	b.h.Rows.Put(hlGroup, "group", hlGroup, json.RawMessage(accJSON(paused)))
	b.mention(1)
	_, s.none = b.take()
	s.states = append(s.states, b.state())
	b.h.Rows.Put(hlGroup, "group", hlGroup, json.RawMessage(accJSON(row)))
	first, _ := b.take()
	b.done(first.Run, "")

	if err := hlSurface(b.t.Context(), b.h, "host.offline"); err != nil {
		return err
	}
	b.mention(2)
	_, held := b.take()
	s.states = append(s.states, b.state())
	if err := hlSurface(b.t.Context(), b.h, "host.online"); err != nil {
		return err
	}
	second, _ := b.take()
	b.done(second.Run, "")

	b.leave()
	time.Sleep(180 * time.Second)
	synctest.Wait()
	b.mention(3)
	s.states = append(s.states, b.state())
	leave := b.h.ExternalPresent(hlGroup, hlAI1)
	defer leave()
	third, _ := b.take()
	s.none = s.none || held
	s.runs = []proto.Handout{first, second, third}
	return nil
}

func runNumbers(runs []proto.Handout) []int64 {
	var out []int64
	for _, r := range runs {
		out = append(out, r.Run)
	}
	return out
}

func (s *extScenario) checkFirst() error {
	h := s.first
	if h.Run != 1 || h.Kind != "group" || h.Task != "" || h.Folder != string(filepath.Separator)+"repo" || h.Text != "items=1" {
		return fmt.Errorf("first hand-out %+v", h)
	}
	if !strings.HasPrefix(h.Context, "instructions of group\n# External run\n") || !strings.Contains(h.Context, "Never wait or poll") {
		return fmt.Errorf("context %q", h.Context)
	}
	return nil
}

func (s *extScenario) checkStates(from int, want ...string) error {
	if got := s.states[from : from+len(want)]; fmt.Sprint(got) != fmt.Sprint(want) {
		return fmt.Errorf("states %v from %d, want %v", s.states, from, want)
	}
	return nil
}

func (s *extScenario) checkSecond() error {
	if err := s.checkStates(1, "waiting"); err != nil {
		return err
	}
	if s.none || s.runs[0].Run != 2 {
		return fmt.Errorf("second run %+v, more handed out %v", s.runs, s.none)
	}
	return nil
}

func (s *extScenario) checkInterrupted(reason, detail string) error {
	for _, c := range s.cmds {
		ev, _ := c["event"].(map[string]any)
		if ev["code"] == "turn_interrupted" && ev["reason"] == reason && (detail == "" || ev["detail"] == detail) {
			return nil
		}
	}
	return fmt.Errorf("no turn_interrupted %s in %v", reason, s.cmds)
}
