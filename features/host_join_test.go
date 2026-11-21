//go:build acceptance

package features

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		return marshalScenarioJSON(proto.ErrorResponse{
			Re:  req.ID,
			Err: proto.WireError{Code: "not_found", Message: "group not found"},
		})
	}
	raw, err := marshalScenarioJSON(grp)
	if err != nil {
		return nil, err
	}
	jh.host.Rows.Put(req.Group, "group", req.Group, raw)
	return marshalScenarioJSON(proto.Synced{
		Type:    "synced",
		Success: proto.Success{Re: req.ID, OK: true},
		Group:   req.Group,
		Seq:     grp.Seq,
	})
}

func (jh *joinScenarioHome) answerCmd(req proto.Cmd) (json.RawMessage, error) {
	var base proto.MemberCommand
	if err := proto.Decode([]byte(req.Env.Payload), &base); err != nil {
		return nil, err
	}
	jh.mu.Lock()
	defer jh.mu.Unlock()
	if base.Op == "add_ai" {
		var add proto.MemberAddAI
		if err := proto.Decode([]byte(req.Env.Payload), &add); err != nil {
			return nil, err
		}
		jh.memberAgents[string(add.ID)] = add.CLI
	}
	if base.Op == "remove" && jh.refuseCode != "" && jh.memberAgents[string(base.ID)] == jh.refuseAgent {
		return marshalScenarioJSON(proto.ErrorResponse{
			Re:  req.ID,
			Err: proto.WireError{Code: jh.refuseCode, Message: "the AI has an unfinished task"},
		})
	}
	return marshalScenarioJSON(proto.CmdResult{Success: proto.Success{Re: req.ID, OK: true}})
}

func marshalScenarioJSON(v any) (json.RawMessage, error) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return json.RawMessage(strings.TrimSuffix(buf.String(), "\n")), nil
}

type joinScenarioWorld struct {
	root          string
	db            *store.Store
	h             *host.Host
	key           identity.Key
	remoteDevice  string
	remoteHome    *joinScenarioHome
	upstreams     map[string]string
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
	raw := make([]byte, 16)
	raw[15] = w.nonceSeq
	return proto.ID(strings.TrimRight(
		strings.NewReplacer("+", "-", "/", "_").Replace(encodeBase64Std(raw)),
		"=",
	))
}

func encodeBase64Std(b []byte) string {
	const table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	out := make([]byte, 24)
	for i := 0; i < 5; i++ {
		v := uint32(b[i*3])<<16 | uint32(b[i*3+1])<<8 | uint32(b[i*3+2])
		out[i*4] = table[(v>>18)&0x3f]
		out[i*4+1] = table[(v>>12)&0x3f]
		out[i*4+2] = table[(v>>6)&0x3f]
		out[i*4+3] = table[v&0x3f]
	}
	v := uint32(b[15]) << 16
	out[20] = table[(v>>18)&0x3f]
	out[21] = table[(v>>12)&0x3f]
	out[22] = '='
	out[23] = '='
	return string(out)
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
	payload, err := marshalScenarioJSON(proto.HostJoin{
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
	})
	if err != nil {
		return err
	}
	_, callErr := host.Surface["host.join"](context.Background(), w.h, nil, "owner-browser", payload)
	return callErr
}

func (w *joinScenarioWorld) leave(agent, groupName string) error {
	gid, ok := joinGroupIDs[groupName]
	if !ok {
		return fmt.Errorf("unknown scenario group %q", groupName)
	}
	payload, err := marshalScenarioJSON(proto.HostLeave{
		HostCommand: proto.HostCommand{
			Type:      "host.leave",
			Nonce:     w.nextNonce(),
			Timestamp: time.Now().UnixMilli(),
			To:        w.h.Key.ID(),
		},
		Group: gid,
		Agent: agent,
	})
	if err != nil {
		return err
	}
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

	sc.Step(`^a paired host on disk with declared agents "([^"]+)" and "([^"]+)" and a remote home "([^"]+)"$`, func(a1, a2, remote string) error {
		tmp, err := os.MkdirTemp("", "solodawn-feat-join-*")
		if err != nil {
			return err
		}
		cleanup(func() { os.RemoveAll(tmp) })
		realRoot, err := filepath.EvalSymlinks(tmp)
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
		w.root = realRoot
		w.db = db
		w.h = h
		w.key = key
		w.remoteDevice = remote
		w.remoteHome = newJoinScenarioHome(h)
		w.upstreams = map[string]string{}
		w.dials = map[string][]context.Context{}
		h.Connect = w.connect

		seed := func(tx *store.Tx) error {
			if _, err := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', '\"owner-user\"')"); err != nil {
				return err
			}
			_, err := tx.Exec("INSERT INTO agent(agent, cli_version, account) VALUES(?, '2.1.311', 'personal'), (?, '0.164.0', 'personal')", a1, a2)
			return err
		}
		return db.Write(seed)
	})

	sc.Step(`^remote home "([^"]+)" hosts project groups "([^"]+)" and "([^"]+)"$`, func(remote, g1, g2 string) error {
		for _, name := range []string{g1, g2} {
			gid, ok := joinGroupIDs[name]
			if !ok {
				return fmt.Errorf("unknown group %q", name)
			}
			repoDir := filepath.Join(w.root, "remotes", strings.ToLower(name))
			if _, err := workspaceCommand(w.root, "init", "-b", "main", repoDir); err != nil {
				return err
			}
			if _, err := workspaceCommand(repoDir, "commit", "--allow-empty", "-m", "initial"); err != nil {
				return err
			}
			w.upstreams[name] = repoDir
			w.remoteHome.groups[string(gid)] = proto.GroupRow{
				RowHeader:      proto.RowHeader{Type: "group", ID: string(gid), Seq: 1},
				Name:           name,
				Kind:           "project",
				Repo:           repoDir,
				Base:           "main",
				Forge:          "none",
				Home:           remote,
				Checks:         []string{},
				Pass:           90,
				PassFoundation: 70,
				Limits: proto.Limits{
					Rounds: 10, WakesPerHour: 20, TurnMinutes: 60, Failures: 3,
					ReviewRounds: 5, AskDepth: 4, AskWaitS: 45, ApprovalWaitS: 600, IdleNoticeMinutes: 30,
				},
				Created: 1_763_600_000_000,
			}
		}
		return nil
	})

	sc.Step(`^the owner joins "([^"]+)" as "([^"]+)" with secretary (true|false) to "([^"]+)" on "([^"]+)"$`, func(agent, name, secStr, groupName, remote string) error {
		if err := w.join(agent, name, secStr == "true", groupName, remote); err != nil {
			return err
		}
		// Remove the upstream repository after its first clone so any second clone attempt would fail.
		if upstream := w.upstreams[groupName]; upstream != "" {
			delete(w.upstreams, groupName)
			return os.RemoveAll(upstream)
		}
		return nil
	})

	sc.Step(`^the host dialled "([^"]+)" (\d+) times? and cloned "([^"]+)" and "([^"]+)" once each$`, func(remote string, wantDials int, g1, g2 string) error {
		w.mu.Lock()
		gotDials := len(w.dials[remote])
		w.mu.Unlock()
		if gotDials != wantDials {
			return fmt.Errorf("dialled %s %d times, want %d", remote, gotDials, wantDials)
		}
		for _, name := range []string{g1, g2} {
			cloneGit := filepath.Join(w.root, "projects", string(joinGroupIDs[name]), "repo", ".git")
			if _, err := os.Stat(cloneGit); err != nil {
				return fmt.Errorf("missing clone for %s: %w", name, err)
			}
			cmd := exec.Command("git", "-C", filepath.Dir(cloneGit), "branch", "--show-current")
			out, err := cmd.CombinedOutput()
			if err != nil || strings.TrimSpace(string(out)) != "main" {
				return fmt.Errorf("clone branch for %s: %q (%v)", name, out, err)
			}
		}
		return nil
	})

	sc.Step(`^the host holds (\d+) dormant group sessions and (\d+) stored homes entries$`, func(wantSessions, wantHomes int) error {
		all := w.h.Sessions.All()
		if len(all) != wantSessions {
			return fmt.Errorf("got %d sessions, want %d", len(all), wantSessions)
		}
		for _, s := range all {
			if s.Kind != host.GroupSession || s.State != host.Dormant || s.Folder != "repo" {
				return fmt.Errorf("unexpected session %+v", s)
			}
		}
		homes, err := w.storedHomes()
		if err != nil {
			return err
		}
		if len(homes) != wantHomes {
			return fmt.Errorf("got %d homes entries (%+v), want %d", len(homes), homes, wantHomes)
		}
		return nil
	})

	sc.Step(`^the host restarts on its store and re-establishes home connections$`, func() error {
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
		ctx, cancel := context.WithCancel(context.Background())
		cleanup(cancel)
		return restarted.EnsureHomeConnections(ctx)
	})

	sc.Step(`^the restarted host dialled "([^"]+)" (\d+) time and holds (\d+) dormant group sessions$`, func(remote string, wantDials, wantSessions int) error {
		w.mu.Lock()
		gotDials := w.restartedDial[remote]
		w.mu.Unlock()
		if gotDials != wantDials {
			return fmt.Errorf("restarted host dialled %s %d times, want %d", remote, gotDials, wantDials)
		}
		all := w.restartedHost.Sessions.All()
		if len(all) != wantSessions {
			return fmt.Errorf("restarted host holds %d sessions, want %d", len(all), wantSessions)
		}
		for _, s := range all {
			if s.State != host.Dormant || s.Kind != host.GroupSession {
				return fmt.Errorf("unexpected restarted session %+v", s)
			}
		}
		return nil
	})

	sc.Step(`^the owner joins undeclared agent "([^"]+)" after clearing its account to "([^"]+)" on "([^"]+)"$`, func(agent, groupName, remote string) error {
		clear := func(tx *store.Tx) error {
			_, err := tx.Exec("UPDATE agent SET account = NULL WHERE agent = ?", agent)
			return err
		}
		if err := w.db.Write(clear); err != nil {
			return err
		}
		w.lastJoinErr = w.join(agent, "Codex", false, groupName, remote)
		return nil
	})

	sc.Step(`^the join command fails with code "([^"]+)" and the host dialled "([^"]+)" (\d+) times$`, func(wantCode, remote string, wantDials int) error {
		var he *host.Error
		if !errors.As(w.lastJoinErr, &he) || he.Code != wantCode {
			return fmt.Errorf("got join error %v, want code %q", w.lastJoinErr, wantCode)
		}
		w.mu.Lock()
		gotDials := len(w.dials[remote])
		w.mu.Unlock()
		if gotDials != wantDials {
			return fmt.Errorf("dialled %s %d times, want %d", remote, gotDials, wantDials)
		}
		return nil
	})

	sc.Step(`^the owner declares "([^"]+)" as "([^"]+)" and joins both "([^"]+)" and "([^"]+)" to "([^"]+)" on "([^"]+)"$`, func(agent, account, first, second, groupName, remote string) error {
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
	})

	sc.Step(`^"([^"]+)" refuses removing "([^"]+)" with "([^"]+)" because of an unfinished task$`, func(_ string, agent, code string) {
		w.remoteHome.mu.Lock()
		w.remoteHome.refuseAgent = agent
		w.remoteHome.refuseCode = code
		w.remoteHome.mu.Unlock()
	})

	sc.Step(`^the owner asks "([^"]+)" to leave "([^"]+)"$`, func(agent, groupName string) {
		w.lastLeaveErr = w.leave(agent, groupName)
	})

	sc.Step(`^the leave command fails with code "([^"]+)" and "([^"]+)" still has (\d+) sessions and an open home connection$`, func(wantCode, groupName string, wantSessions int) error {
		var he *host.Error
		if !errors.As(w.lastLeaveErr, &he) || he.Code != wantCode {
			return fmt.Errorf("got leave error %v, want code %q", w.lastLeaveErr, wantCode)
		}
		return w.checkGroupStatus(groupName, wantSessions, true, true)
	})

	sc.Step(`^"([^"]+)" accepts member removals and "([^"]+)" leaves "([^"]+)"$`, func(_ string, agent, groupName string) error {
		w.remoteHome.mu.Lock()
		w.remoteHome.refuseAgent = ""
		w.remoteHome.refuseCode = ""
		w.remoteHome.mu.Unlock()
		return w.leave(agent, groupName)
	})

	sc.Step(`^"([^"]+)" has (\d+) session and remains in stored homes with its home connection open$`, func(groupName string, wantSessions int) error {
		return w.checkGroupStatus(groupName, wantSessions, true, true)
	})

	sc.Step(`^"([^"]+)" leaves "([^"]+)"$`, func(agent, groupName string) error {
		return w.leave(agent, groupName)
	})

	sc.Step(`^"([^"]+)" has (\d+) sessions, stored homes is empty, and the connection to "([^"]+)" is closed$`, func(groupName string, wantSessions int, remote string) error {
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
	})
}

func (w *joinScenarioWorld) checkGroupStatus(groupName string, wantSessions int, wantInHomes, wantConnOpen bool) error {
	gid := string(joinGroupIDs[groupName])
	count := 0
	for _, s := range w.h.Sessions.All() {
		if s.Group == gid {
			count++
		}
	}
	if count != wantSessions {
		return fmt.Errorf("group %s has %d sessions, want %d", groupName, count, wantSessions)
	}
	homes, err := w.storedHomes()
	if err != nil {
		return err
	}
	inHomes := false
	for _, h := range homes {
		if string(h.Group) == gid {
			inHomes = true
		}
	}
	if inHomes != wantInHomes {
		return fmt.Errorf("group %s in stored homes = %v, want %v (homes=%+v)", groupName, inHomes, wantInHomes, homes)
	}
	w.mu.Lock()
	ctxs := w.dials[w.remoteDevice]
	w.mu.Unlock()
	if len(ctxs) == 0 {
		return errors.New("expected at least one dial")
	}
	open := ctxs[len(ctxs)-1].Err() == nil
	if open != wantConnOpen {
		return fmt.Errorf("home connection open = %v, want %v", open, wantConnOpen)
	}
	return nil
}
