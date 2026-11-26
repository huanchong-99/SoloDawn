//go:build acceptance

package features

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_join"] = hostJoinSteps
}

var joinGroupIDs = map[string]proto.ID{
	"Website": "rZNvy-1jH6Z-BcPqA5U5BQ",
	"Backend": "H46nb5YsBesaWbHkHmZLow",
}

type joinScenarioHome struct {
	mu           sync.Mutex
	done         chan struct{}
	host         *host.Host
	groups       map[string]proto.GroupRow
	refuseAgent  string
	refuseCode   string
	memberAgents map[string]string
}

func newJoinScenarioHome(h *host.Host) *joinScenarioHome {
	return &joinScenarioHome{
		done:         make(chan struct{}),
		host:         h,
		groups:       map[string]proto.GroupRow{},
		memberAgents: map[string]string{},
	}
}

func (jh *joinScenarioHome) Send(any) error               { return nil }
func (jh *joinScenarioHome) Path() string                 { return "direct" }
func (jh *joinScenarioHome) PathChanged() <-chan struct{} { return nil }
func (jh *joinScenarioHome) Done() <-chan struct{}        { return jh.done }

func (jh *joinScenarioHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	frame := build(11)
	switch req := frame.(type) {
	case proto.Sync:
		return jh.answerSync(req)
	case proto.Cmd:
		return jh.answerCmd(req)
	default:
		return nil, io.EOF
	}
}

func (jh *joinScenarioHome) answerSync(req proto.Sync) (json.RawMessage, error) {
	jh.mu.Lock()
	grp, ok := jh.groups[req.Group]
	jh.mu.Unlock()
	if !ok {
		return json.RawMessage(accJSON(proto.ErrorResponse{
			Re:  req.ID,
			Err: proto.WireError{Code: "not_found", Message: "group not found"},
		})), nil
	}
	jh.host.Rows.Put(req.Group, "group", req.Group, json.RawMessage(accJSON(grp)))
	return json.RawMessage(accJSON(proto.Synced{
		Type:    "synced",
		Success: proto.Success{Re: req.ID, OK: true},
		Group:   req.Group,
		Seq:     grp.Seq,
	})), nil
}

func (jh *joinScenarioHome) answerCmd(req proto.Cmd) (json.RawMessage, error) {
	var base proto.MemberCommand
	if err := proto.Decode([]byte(req.Env.Payload), &base); err != nil {
		return nil, err
	}
	jh.mu.Lock()
	defer jh.mu.Unlock()
	if base.Op == "add_ai" {
		return jh.answerAddAI(req.ID, req.Env.Payload)
	}
	if jh.shouldRefuseRemove(base) {
		return json.RawMessage(accJSON(proto.ErrorResponse{
			Re:  req.ID,
			Err: proto.WireError{Code: jh.refuseCode, Message: "the AI has an unfinished task"},
		})), nil
	}
	return json.RawMessage(accJSON(proto.CmdResult{Success: proto.Success{Re: req.ID, OK: true}})), nil
}

func (jh *joinScenarioHome) answerAddAI(re int64, payload string) (json.RawMessage, error) {
	var add proto.MemberAddAI
	if err := proto.Decode([]byte(payload), &add); err != nil {
		return nil, err
	}
	jh.memberAgents[string(add.ID)] = add.CLI
	return json.RawMessage(accJSON(proto.CmdResult{Success: proto.Success{Re: re, OK: true}})), nil
}

func (jh *joinScenarioHome) shouldRefuseRemove(base proto.MemberCommand) bool {
	return base.Op == "remove" && jh.refuseCode != "" && jh.memberAgents[string(base.ID)] == jh.refuseAgent
}

type joinScenarioWorld struct {
	root          string
	db            *store.Store
	h             *host.Host
	key           identity.Key
	remoteDevice  string
	remoteHome    *joinScenarioHome
	mu            sync.Mutex
	dials         map[string][]context.Context
	restartedHost *host.Host
	restartedDial map[string]int
	lastJoinErr   error
	lastLeaveErr  error
	nonceSeq      byte
}

func (w *joinScenarioWorld) nextNonce() proto.ID {
	w.nonceSeq++
	var raw [16]byte
	raw[15] = w.nonceSeq
	return proto.ID(base64.RawURLEncoding.EncodeToString(raw[:]))
}

func (w *joinScenarioWorld) connect(ctx context.Context, device string) {
	w.mu.Lock()
	w.dials[device] = append(w.dials[device], ctx)
	jh := w.remoteHome
	match := device == w.remoteDevice
	w.mu.Unlock()
	if match && jh != nil {
		w.h.Attach(device, jh)
	}
}

func (w *joinScenarioWorld) join(agent, name string, secretary bool, groupName, homeDevice string) error {
	gid, ok := joinGroupIDs[groupName]
	if !ok {
		return fmt.Errorf("unknown scenario group %q", groupName)
	}
	workspace, err := aiWorkspace(w.root, gid, name)
	if err != nil {
		return err
	}
	payload := json.RawMessage(accJSON(proto.HostJoin{
		HostCommand: proto.HostCommand{
			Type:      "host.join",
			Nonce:     w.nextNonce(),
			Timestamp: time.Now().UnixMilli(),
			To:        w.h.Key.ID(),
		},
		Group:     gid,
		Home:      homeDevice,
		Agent:     agent,
		Name:      name,
		Secretary: secretary,
		Workspace: workspace,
	}))
	_, callErr := host.Surface["host.join"](context.Background(), w.h, nil, "owner-browser", payload)
	return callErr
}

func (w *joinScenarioWorld) leave(agent, groupName string) error {
	gid, ok := joinGroupIDs[groupName]
	if !ok {
		return fmt.Errorf("unknown scenario group %q", groupName)
	}
	payload := json.RawMessage(accJSON(proto.HostLeave{
		HostCommand: proto.HostCommand{
			Type:      "host.leave",
			Nonce:     w.nextNonce(),
			Timestamp: time.Now().UnixMilli(),
			To:        w.h.Key.ID(),
		},
		Group: gid,
		Agent: agent,
	}))
	_, callErr := host.Surface["host.leave"](context.Background(), w.h, nil, "owner-browser", payload)
	return callErr
}

func (w *joinScenarioWorld) storedHomes() ([]proto.Home, error) {
	var raw string
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT coalesce((SELECT value FROM setting WHERE key = 'homes'), '[]')").Scan(&raw)
	}
	if err := w.db.Write(read); err != nil {
		return nil, err
	}
	var homes []proto.Home
	if err := json.Unmarshal([]byte(raw), &homes); err != nil {
		return nil, err
	}
	return homes, nil
}

func hostJoinSteps(sc *godog.ScenarioContext, _ fixtures) {
	w := &joinScenarioWorld{}
	cleanup := afterScenario(sc)
	registerJoinSetupSteps(sc, w, cleanup)
	registerJoinAssertionSteps(sc, w)
	registerLeaveScenarioSteps(sc, w)
}

func registerJoinSetupSteps(sc *godog.ScenarioContext, w *joinScenarioWorld, cleanup func(func())) {
	sc.Step(`^a paired host on disk with declared agents "([^"]+)" and "([^"]+)" and a remote home "([^"]+)"$`, func(a1, a2, remote string) error {
		return w.initHostOnDisk(a1, a2, remote, cleanup)
	})
	sc.Step(`^remote home "([^"]+)" hosts project groups "([^"]+)" and "([^"]+)"$`, func(remote, g1, g2 string) error {
		for _, name := range []string{g1, g2} {
			if err := w.createRemoteGroup(remote, name); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^the owner joins "([^"]+)" as "([^"]+)" with secretary (true|false) to "([^"]+)" on "([^"]+)"$`, func(agent, name, secStr, groupName, remote string) error {
		return w.join(agent, name, secStr == "true", groupName, remote)
	})
	sc.Step(`^the host restarts on its store and re-establishes home connections$`, func() error {
		return w.restartAndReconnect()
	})
}

func (w *joinScenarioWorld) initHostOnDisk(a1, a2, remote string, cleanup func(func())) error {
	realRoot, err := makeScenarioRoot(cleanup)
	if err != nil {
		return err
	}
	db, err := store.Open(filepath.Join(realRoot, "node.db"))
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	key := identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)))
	h, _, err := host.New(db, key, time.Now)
	if err != nil {
		return err
	}
	w.root, w.db, w.h, w.key = realRoot, db, h, key
	w.remoteDevice, w.remoteHome = remote, newJoinScenarioHome(h)
	w.dials = map[string][]context.Context{}
	h.Connect = w.connect
	return seedScenarioAgents(db, a1, a2)
}

func makeScenarioRoot(cleanup func(func())) (string, error) {
	tmp, err := os.MkdirTemp("", "solodawn-feat-join-*")
	if err != nil {
		return "", err
	}
	cleanup(func() { os.RemoveAll(tmp) })
	return filepath.EvalSymlinks(tmp)
}

func seedScenarioAgents(db *store.Store, a1, a2 string) error {
	seed := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO agent(agent, cli_version, account) VALUES(?, '2.1.311', 'personal'), (?, '0.164.0', 'personal')", a1, a2)
		return err
	}
	return db.Write(seed)
}

func (w *joinScenarioWorld) createRemoteGroup(remote, name string) error {
	gid, ok := joinGroupIDs[name]
	if !ok {
		return fmt.Errorf("unknown group %q", name)
	}
	w.remoteHome.groups[string(gid)] = proto.GroupRow{
		RowHeader:      proto.RowHeader{Type: "group", ID: string(gid), Seq: 1},
		Name:           name,
		Home:           remote,
		Pass:           90,
		PassFoundation: 70,
		Limits: proto.Limits{
			Rounds: 10, WakesPerHour: 20, TurnMinutes: 60, Failures: 3,
			ReviewRounds: 5, AskDepth: 4, AskWaitS: 45, ApprovalWaitS: 600, IdleNoticeMinutes: 30,
		},
		Created: 1_763_600_000_000,
	}
	return nil
}

func (w *joinScenarioWorld) restartAndReconnect() error {
	restarted, _, err := host.New(w.db, w.key, time.Now)
	if err != nil {
		return err
	}
	w.restartedHost = restarted
	w.restartedDial = map[string]int{}
	restarted.Connect = func(_ context.Context, device string) {
		w.mu.Lock()
		w.restartedDial[device]++
		w.mu.Unlock()
	}
	return restarted.EnsureHomeConnections()
}

func registerJoinAssertionSteps(sc *godog.ScenarioContext, w *joinScenarioWorld) {
	sc.Step(`^the host dialled "([^"]+)" (\d+) times? and cloned "([^"]+)" and "([^"]+)" once each$`, rewritten("AC-2", "a join records the workspace of the AI and clones nothing (MODEL-1126 plan 3.2: host_join.feature)"))
	sc.Step(`^the host holds (\d+) dormant group sessions and (\d+) stored homes entries$`, func(wantSessions, wantHomes int) error {
		if err := checkDormantGroupSessions(w.h.Sessions.All(), wantSessions); err != nil {
			return err
		}
		homes, err := w.storedHomes()
		if err != nil || len(homes) != wantHomes {
			return fmt.Errorf("got homes=%+v (err=%v), want %d", homes, err, wantHomes)
		}
		return nil
	})
	sc.Step(`^the restarted host dialled "([^"]+)" (\d+) time and holds (\d+) dormant group sessions$`, func(remote string, wantDials, wantSessions int) error {
		w.mu.Lock()
		gotDials := w.restartedDial[remote]
		w.mu.Unlock()
		if gotDials != wantDials {
			return fmt.Errorf("restarted host dialled %s %d times, want %d", remote, gotDials, wantDials)
		}
		return checkDormantGroupSessions(w.restartedHost.Sessions.All(), wantSessions)
	})
}

func (w *joinScenarioWorld) checkDialCount(remote string, wantDials int) error {
	w.mu.Lock()
	gotDials := len(w.dials[remote])
	w.mu.Unlock()
	if gotDials != wantDials {
		return fmt.Errorf("dialled %s %d times, want %d", remote, gotDials, wantDials)
	}
	return nil
}

func checkDormantGroupSessions(all []host.Session, want int) error {
	if len(all) != want {
		return fmt.Errorf("got %d sessions, want %d", len(all), want)
	}
	for _, s := range all {
		if !isDormantGroupRepoSession(s) {
			return fmt.Errorf("unexpected session %+v", s)
		}
	}
	return nil
}

func isDormantGroupRepoSession(s host.Session) bool {
	return s.Kind == host.GroupSession && s.State == host.Dormant && s.Folder == "repo"
}

func registerLeaveScenarioSteps(sc *godog.ScenarioContext, w *joinScenarioWorld) {
	sc.Step(`^the owner joins undeclared agent "([^"]+)" after clearing its account to "([^"]+)" on "([^"]+)"$`, func(agent, groupName, remote string) error {
		return w.clearAccountAndAttemptJoin(agent, groupName, remote)
	})
	sc.Step(`^the join command fails with code "([^"]+)" and the host dialled "([^"]+)" (\d+) times$`, func(wantCode, remote string, wantDials int) error {
		if err := checkHostErrorCode(w.lastJoinErr, wantCode); err != nil {
			return err
		}
		return w.checkDialCount(remote, wantDials)
	})
	sc.Step(`^the owner declares "([^"]+)" as "([^"]+)" and joins both "([^"]+)" and "([^"]+)" to "([^"]+)" on "([^"]+)"$`, func(agent, account, first, second, groupName, remote string) error {
		return w.declareAndJoinTwo(agent, account, first, second, groupName, remote)
	})
	sc.Step(`^"([^"]+)" refuses removing "([^"]+)" with "([^"]+)" because of an unfinished task$`, func(_ string, agent, code string) {
		w.setRemoveRefusal(agent, code)
	})
	sc.Step(`^the owner asks "([^"]+)" to leave "([^"]+)"$`, func(agent, groupName string) {
		w.lastLeaveErr = w.leave(agent, groupName)
	})
	sc.Step(`^the leave command fails with code "([^"]+)" and "([^"]+)" still has (\d+) sessions and an open home connection$`, func(wantCode, groupName string, wantSessions int) error {
		if err := checkHostErrorCode(w.lastLeaveErr, wantCode); err != nil {
			return err
		}
		return w.checkGroupStatus(groupName, wantSessions, true, true)
	})
	sc.Step(`^"([^"]+)" accepts member removals and "([^"]+)" leaves "([^"]+)"$`, func(_ string, agent, groupName string) error {
		w.setRemoveRefusal("", "")
		return w.leave(agent, groupName)
	})
	sc.Step(`^"([^"]+)" has (\d+) session and remains in stored homes with its home connection open$`, func(groupName string, wantSessions int) error {
		return w.checkGroupStatus(groupName, wantSessions, true, true)
	})
	sc.Step(`^"([^"]+)" leaves "([^"]+)"$`, func(agent, groupName string) error {
		return w.leave(agent, groupName)
	})
	sc.Step(`^"([^"]+)" has (\d+) sessions, stored homes is empty, and the connection to "([^"]+)" is closed$`, func(groupName string, wantSessions int, remote string) error {
		return w.checkFinalClosedStatus(groupName, wantSessions, remote)
	})
}

func (w *joinScenarioWorld) clearAccountAndAttemptJoin(agent, groupName, remote string) error {
	clear := func(tx *store.Tx) error {
		_, err := tx.Exec("UPDATE agent SET account = NULL WHERE agent = ?", agent)
		return err
	}
	if err := w.db.Write(clear); err != nil {
		return err
	}
	w.lastJoinErr = w.join(agent, "Codex", false, groupName, remote)
	return nil
}

func (w *joinScenarioWorld) declareAndJoinTwo(agent, account, first, second, groupName, remote string) error {
	declare := func(tx *store.Tx) error {
		_, err := tx.Exec("UPDATE agent SET account = ? WHERE agent = ?", account, agent)
		return err
	}
	if err := w.db.Write(declare); err != nil {
		return err
	}
	if err := w.join(first, "Claude", true, groupName, remote); err != nil {
		return err
	}
	return w.join(second, "Codex", false, groupName, remote)
}

func (w *joinScenarioWorld) setRemoveRefusal(agent, code string) {
	w.remoteHome.mu.Lock()
	w.remoteHome.refuseAgent = agent
	w.remoteHome.refuseCode = code
	w.remoteHome.mu.Unlock()
}

func checkHostErrorCode(err error, wantCode string) error {
	var he *host.Error
	if !errors.As(err, &he) || he.Code != wantCode {
		return fmt.Errorf("got error %v, want code %q", err, wantCode)
	}
	return nil
}

func (w *joinScenarioWorld) checkFinalClosedStatus(groupName string, wantSessions int, remote string) error {
	if err := w.checkGroupStatus(groupName, wantSessions, false, false); err != nil {
		return err
	}
	w.mu.Lock()
	ctxs := w.dials[remote]
	w.mu.Unlock()
	if len(ctxs) != 1 || ctxs[0].Err() == nil {
		return fmt.Errorf("expected single dial to %s with cancelled context, got %d dials", remote, len(ctxs))
	}
	return nil
}

func (w *joinScenarioWorld) checkGroupStatus(groupName string, wantSessions int, wantInHomes, wantConnOpen bool) error {
	gid := string(joinGroupIDs[groupName])
	if got := countGroupSessions(w.h.Sessions.All(), gid); got != wantSessions {
		return fmt.Errorf("group %s has %d sessions, want %d", groupName, got, wantSessions)
	}
	if err := w.checkGroupInHomes(groupName, gid, wantInHomes); err != nil {
		return err
	}
	return w.checkRemoteConnOpen(wantConnOpen)
}

func countGroupSessions(all []host.Session, gid string) int {
	count := 0
	for _, s := range all {
		if s.Group == gid {
			count++
		}
	}
	return count
}

func (w *joinScenarioWorld) checkGroupInHomes(groupName, gid string, want bool) error {
	homes, err := w.storedHomes()
	if err != nil {
		return err
	}
	found := false
	for _, h := range homes {
		if string(h.Group) == gid {
			found = true
		}
	}
	if found != want {
		return fmt.Errorf("group %s in stored homes = %v, want %v (homes=%+v)", groupName, found, want, homes)
	}
	return nil
}

func (w *joinScenarioWorld) checkRemoteConnOpen(wantOpen bool) error {
	w.mu.Lock()
	ctxs := w.dials[w.remoteDevice]
	w.mu.Unlock()
	if len(ctxs) == 0 {
		return errors.New("expected at least one dial")
	}
	open := ctxs[len(ctxs)-1].Err() == nil
	if open != wantOpen {
		return fmt.Errorf("home connection open = %v, want %v", open, wantOpen)
	}
	return nil
}
