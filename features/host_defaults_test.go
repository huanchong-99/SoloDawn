//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_defaults"] = hostDefaultsSteps
}

type defaultsHome struct {
	key          identity.Key
	ownerKey     identity.Key
	hostID       string
	db           *store.Store
	loop         *home.Loop
	mu           sync.Mutex
	disconnected bool
	links        []*defaultsHomeLink
	hostSetCmds  []proto.MemberSet
}

func newDefaultsHome(cleanup func(func()), ownerKey, homeKey identity.Key, allKeys []identity.Key, hostID string, clock func() time.Time) (*defaultsHome, error) {
	db, err := store.Open(":memory:")
	if err != nil {
		return nil, err
	}
	cleanup(func() { db.Close() })
	hn := &defaultsHome{key: homeKey, ownerKey: ownerKey, hostID: hostID, db: db}
	tables := home.NodeTables()
	tables.Commands = maps.Clone(tables.Commands)
	origMember := tables.Commands["member/set"]
	tables.Commands["member/set"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		var ms proto.MemberSet
		if c.Decode(&ms) == nil && ms.Op == "set" && c.Who.Device == hn.hostID {
			hn.mu.Lock()
			hn.hostSetCmds = append(hn.hostSetCmds, ms)
			hn.mu.Unlock()
		}
		return origMember(s, c)
	}
	log, err := homeLog(allKeys)
	if err != nil {
		return nil, err
	}
	loop, err := home.StartLoop(home.LoopConfig{
		Store:  db,
		Home:   homeKey.ID(),
		Owner:  ownerKey.ID(),
		Logs:   func(string) *identity.Log { return log },
		Tables: tables,
		Now:    clock,
	})
	if err != nil {
		return nil, err
	}
	cleanup(loop.Close)
	hn.loop = loop
	return hn, nil
}

func (hn *defaultsHome) attachHost(h *host.Host) {
	hn.mu.Lock()
	if hn.disconnected {
		hn.mu.Unlock()
		return
	}
	link := &defaultsHomeLink{
		host:    h,
		home:    hn,
		done:    make(chan struct{}),
		waiting: map[int64]chan json.RawMessage{},
	}
	hn.links = append(hn.links, link)
	hn.mu.Unlock()

	link.conn = hn.loop.Attach(hn.hostID, defaultsHomeClient{link})
	h.Attach(hn.key.ID(), link)
	hn.loop.Inbox(link.conn)
}

func (hn *defaultsHome) disconnect() {
	hn.mu.Lock()
	hn.disconnected = true
	links := append([]*defaultsHomeLink(nil), hn.links...)
	hn.links = nil
	hn.mu.Unlock()
	for _, l := range links {
		l.closeLink()
	}
}

func (hn *defaultsHome) reconnect(h *host.Host) {
	hn.mu.Lock()
	hn.disconnected = false
	hn.mu.Unlock()
	hn.attachHost(h)
}

func (hn *defaultsHome) hostSetsFor(memberID string) []proto.MemberSet {
	hn.mu.Lock()
	defer hn.mu.Unlock()
	var out []proto.MemberSet
	for _, cmd := range hn.hostSetCmds {
		if string(cmd.ID) == memberID {
			out = append(out, cmd)
		}
	}
	return out
}

func (hn *defaultsHome) activePushes() int64 {
	hn.mu.Lock()
	defer hn.mu.Unlock()
	var total int64
	for _, l := range hn.links {
		total += l.active.Load()
	}
	return total
}

func (hn *defaultsHome) sendOwnerCmd(payload any) (proto.CmdResult, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return proto.CmdResult{}, err
	}
	env := hn.ownerKey.Sign(string(raw))
	ch := make(chan json.RawMessage, 1) // single reader: <-ch below
	conn := hn.loop.Attach(hn.ownerKey.ID(), defaultsCmdClient{ch: ch})
	defer hn.loop.Detach(conn)
	hn.loop.Cmd(conn, 1, env)
	select {
	case data := <-ch:
		var errResp proto.ErrorResponse
		if proto.Decode(data, &errResp) == nil && errResp.Err.Code != "" {
			return proto.CmdResult{}, &host.Error{Code: errResp.Err.Code, Message: errResp.Err.Message}
		}
		var res proto.CmdResult
		return res, proto.Decode(data, &res)
	case <-time.After(5 * time.Second):
		return proto.CmdResult{}, errors.New("timed out waiting for home command reply; saw no answer")
	}
}

func (hn *defaultsHome) memberRows(groupID string) ([]proto.AIMemberRow, error) {
	var out []proto.AIMemberRow
	read := func(tx *store.Tx) error {
		rows, err := tx.Query(`SELECT data FROM row WHERE g = ? AND r = 'member' AND gone = 0`, groupID)
		if err != nil {
			return err
		}
		defer rows.Close()
		return scanAIMemberRows(rows, &out)
	}
	return out, hn.db.Write(read)
}

func scanAIMemberRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}, out *[]proto.AIMemberRow) error {
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		ai, ok, err := decodeAIMemberRow(raw)
		if err != nil {
			return err
		}
		if ok {
			*out = append(*out, ai)
		}
	}
	return rows.Err()
}

func decodeAIMemberRow(raw string) (proto.AIMemberRow, bool, error) {
	var hdr struct {
		Kind string `json:"kind"`
	}
	if proto.Decode([]byte(raw), &hdr) != nil || hdr.Kind != "ai" {
		return proto.AIMemberRow{}, false, nil
	}
	var ai proto.AIMemberRow
	err := proto.Decode([]byte(raw), &ai)
	return ai, err == nil, err
}

type defaultsCmdClient struct{ ch chan json.RawMessage }

func (c defaultsCmdClient) Done() <-chan struct{} { return nil }
func (c defaultsCmdClient) Send(frame any) error {
	data, err := defaultsEncodeJSON(frame)
	if err != nil {
		return err
	}
	c.ch <- data
	return nil
}

type defaultsHomeClient struct{ link *defaultsHomeLink }

func (c defaultsHomeClient) Done() <-chan struct{} { return c.link.Done() }
func (c defaultsHomeClient) Send(frame any) error {
	data, err := defaultsEncodeJSON(frame)
	if err != nil {
		return err
	}
	return c.link.deliverFromHome(data)
}

type defaultsHomeLink struct {
	host    *host.Host
	home    *defaultsHome
	conn    *home.Conn
	done    chan struct{}
	pumping sync.WaitGroup
	active  atomic.Int64
	mu      sync.Mutex
	closed  bool
	next    int64
	waiting map[int64]chan json.RawMessage
}

func (l *defaultsHomeLink) Path() string                 { return "direct" }
func (l *defaultsHomeLink) PathChanged() <-chan struct{} { return nil }
func (l *defaultsHomeLink) Done() <-chan struct{}        { return l.done }

func (l *defaultsHomeLink) deliverFromHome(data []byte) error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return io.EOF
	}
	l.pumping.Add(1)
	l.mu.Unlock()
	l.active.Add(1)
	defer l.active.Add(-1)
	defer l.pumping.Done()
	l.fromHome(data)
	return nil
}

func (l *defaultsHomeLink) closeLink() {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return
	}
	l.closed = true
	close(l.done)
	l.mu.Unlock()
	if l.conn != nil {
		l.home.loop.Detach(l.conn)
	}
	l.pumping.Wait()
}

func (l *defaultsHomeLink) Send(frame any) error {
	l.mu.Lock()
	closed := l.closed
	l.mu.Unlock()
	if closed {
		return io.EOF
	}
	data, err := defaultsEncodeJSON(frame)
	if err != nil {
		return err
	}
	l.toHome(data)
	return nil
}

func (l *defaultsHomeLink) Request(ctx context.Context, build func(int64) any) (json.RawMessage, error) {
	id, reply := l.openWait()
	defer l.dropWait(id)
	if err := l.Send(build(id)); err != nil {
		return nil, err
	}
	select {
	case answer := <-reply:
		return answer, nil
	case <-l.Done():
		return nil, io.EOF
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (l *defaultsHomeLink) openWait() (int64, chan json.RawMessage) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.next++
	reply := make(chan json.RawMessage, 1) // single reader: Request call owning l.next
	l.waiting[l.next] = reply
	return l.next, reply
}

func (l *defaultsHomeLink) dropWait(id int64) {
	l.mu.Lock()
	delete(l.waiting, id)
	l.mu.Unlock()
}

var defaultsToHome = map[string]func(*defaultsHomeLink, []byte){
	"cmd": func(l *defaultsHomeLink, data []byte) {
		var cmd proto.Cmd
		if proto.Decode(data, &cmd) == nil {
			l.home.loop.Cmd(l.conn, cmd.ID, cmd.Env)
		}
	},
	"sync": func(l *defaultsHomeLink, data []byte) {
		var req proto.Sync
		if proto.Decode(data, &req) == nil {
			l.home.loop.Sync(l.conn, req)
		}
	},
	"fetch": func(l *defaultsHomeLink, data []byte) {
		if freq, ok := defaultsDecodeFetch(data); ok {
			l.home.loop.Fetch(l.conn, freq)
		}
	},
	"ack": func(l *defaultsHomeLink, data []byte) {
		var ack proto.Ack
		if proto.Decode(data, &ack) == nil {
			l.home.loop.Ack(l.conn, ack)
		}
	},
}

func (l *defaultsHomeLink) toHome(data []byte) {
	var hdr struct {
		Type string `json:"t"`
	}
	if json.Unmarshal(data, &hdr) != nil {
		return
	}
	if fn := defaultsToHome[hdr.Type]; fn != nil {
		fn(l, data)
	}
}

func defaultsDecodeFetch(data []byte) (home.Fetch, bool) {
	var th proto.FetchThread
	if proto.Decode(data, &th) == nil {
		return home.Fetch{FetchPage: th.FetchPage, Thread: th.Thread}, th.Type == "fetch"
	}
	var req home.Fetch
	return req, proto.Decode(data, &req) == nil && req.Type == "fetch"
}

func (l *defaultsHomeLink) fromHome(data []byte) {
	var hdr struct {
		Re *int64 `json:"re,omitempty"`
	}
	if proto.Decode(data, &hdr) != nil {
		return
	}
	if hdr.Re != nil {
		l.mu.Lock()
		ch := l.waiting[*hdr.Re]
		delete(l.waiting, *hdr.Re)
		l.mu.Unlock()
		if ch != nil {
			ch <- data
		}
		return
	}
	l.host.Push(l.home.key.ID(), data)
}

func defaultsEncodeJSON(frame any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	err := enc.Encode(frame)
	return bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}), err
}

func defaultsSeedKey(b byte) identity.Key {
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = b
	return identity.Key(ed25519.NewKeyFromSeed(seed))
}

func defaultsID(n uint64) proto.ID {
	var raw [16]byte
	binary.BigEndian.PutUint64(raw[8:], n)
	return proto.ID(base64.RawURLEncoding.EncodeToString(raw[:]))
}

type defaultsWorld struct {
	f            fixtures
	cleanup      func(func())
	root         string
	ownerKey     identity.Key
	hostKey      identity.Key
	homeKeys     []identity.Key
	db           *store.Store
	h            *host.Host
	sessions     *node.Sessions
	homes        map[string]*defaultsHome // by home device id
	groups       map[string]proto.ID      // by group name
	grpHomes     map[string]*defaultsHome // by group name
	members      map[string]string        // by "<group>:<name>"
	seq          uint64
	nowMS        int64
	wantDefaults int
}

func hostDefaultsSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &defaultsWorld{f: f, cleanup: afterScenario(sc)}
	sc.Step(`^a node with claude found and its account declared$`, func() error {
		return w.initNode("claude", false)
	})
	sc.Step(`^host\.info lists claude without defaults$`, func() error { return w.checkHostInfoDefaults("claude", nil) })
	sc.Step(`^the owner sends agent\.defaults for (claude|codex) with "([^"]*)", "([^"]*)", "([^"]*)" and "([^"]*)"$`, w.sendAgentDefaults)
	sc.Step(`^host\.info lists claude with defaults "([^"]*)", "([^"]*)", "([^"]*)" and "([^"]*)"$`, func(m, e, t, s string) error {
		return w.checkHostInfoDefaults("claude", &proto.Profile{Model: m, Effort: e, Tier: t, Skills: s})
	})
	sc.Step(`^agent\.defaults for "([^"]*)" is refused with "([^"]*)"$`, w.checkDefaultsBadAgent)
	sc.Step(`^agent\.defaults with tier "([^"]*)" is refused with "([^"]*)"$`, w.checkDefaultsBadTier)
	sc.Step(`^agent\.defaults without skills is refused with "([^"]*)"$`, w.checkDefaultsMissingSkills)

	sc.Step(`^claude defaults "([^"]*)", "([^"]*)", "([^"]*)" and "([^"]*)" on the node$`, w.givenClaudeDefaults)
	sc.Step(`^the owner joins (claude|codex) to "([^"]*)" following the defaults$`, func(agent, group string) error {
		return w.joinGroup(agent, group, agent, proto.ProfileSet{}, true)
	})
	sc.Step(`^the home has a claude member in "([^"]*)" that follows, with "([^"]*)", "([^"]*)", "([^"]*)" and "([^"]*)"$`, func(group, m, e, t, s string) error {
		return w.checkHomeMember(group, "claude", true, proto.Profile{Model: m, Effort: e, Tier: t, Skills: s})
	})
	sc.Step(`^the owner joins (claude|codex) to "([^"]*)" again as "([^"]*)" with model "([^"]*)" and tier "([^"]*)"$`, func(agent, group, name, model, tier string) error {
		return w.joinGroup(agent, group, name, proto.ProfileSet{Model: &model, Tier: &tier}, false)
	})
	sc.Step(`^the "([^"]*)" member has its own profile "([^"]*)", "([^"]*)", "([^"]*)" and "([^"]*)"$`, func(name, m, e, t, s string) error {
		return w.checkHomeMember("Website", name, false, proto.Profile{Model: m, Effort: e, Tier: t, Skills: s})
	})

	sc.Step(`^claude on the node is a member of "([^"]*)" and "([^"]*)" following the defaults, homed on two nodes$`, w.givenTwoHomeFollowers)
	sc.Step(`^claude is a member of "([^"]*)" with its own profile "([^"]*)", "([^"]*)", "([^"]*)" and "([^"]*)"$`, w.givenOwnClaudeMember)
	sc.Step(`^codex on the node is a member of "([^"]*)" following the defaults$`, func(group string) error {
		return w.joinGroup("codex", group, "Codex", proto.ProfileSet{}, true)
	})
	sc.Step(`^the claude members of "([^"]*)" and "([^"]*)" have "([^"]*)", "([^"]*)", "([^"]*)" and "([^"]*)"$`, func(g1, g2, m, e, t, s string) error {
		want := proto.Profile{Model: m, Effort: e, Tier: t, Skills: s}
		if err := w.waitHomeMember(g1, "claude", true, want); err != nil {
			return err
		}
		return w.waitHomeMember(g2, "claude", true, want)
	})
	sc.Step(`^the claude member of "([^"]*)" keeps "([^"]*)", "([^"]*)", "([^"]*)" and "([^"]*)"$`, func(g, m, e, t, s string) error {
		return w.checkHomeMember(g, "claude-own", false, proto.Profile{Model: m, Effort: e, Tier: t, Skills: s})
	})
	sc.Step(`^no member set was sent for the codex member$`, func() error {
		if err := w.settleWork(); err != nil {
			return err
		}
		id := w.members["Website:Codex"]
		got := w.grpHomes["Website"].hostSetsFor(id)
		if len(got) != 0 {
			return fmt.Errorf("expected 0 member set commands for codex (%s), got %d (%+v)", id, len(got), got)
		}
		return nil
	})

	sc.Step(`^claude on the node is a member of "([^"]*)" following the defaults$`, w.givenSingleAppFollower)
	sc.Step(`^the home of "([^"]*)" is disconnected$`, w.disconnectHome)
	sc.Step(`^the home of "([^"]*)" connects again$`, w.reconnectHome)
	sc.Step(`^after the sync the claude member of "([^"]*)" has "([^"]*)", "([^"]*)", "([^"]*)" and "([^"]*)"$`, func(g, m, e, t, s string) error {
		return w.waitHomeMember(g, "claude", true, proto.Profile{Model: m, Effort: e, Tier: t, Skills: s})
	})

	sc.Step(`^the owner sets follow on that member at the home$`, w.ownerSetsFollowOnDocs)
	sc.Step(`^the host sends exactly one member set for it, with "([^"]*)", "([^"]*)", "([^"]*)" and "([^"]*)"$`, w.checkSingleHostRestore)
}

func (w *defaultsWorld) nextID() proto.ID {
	w.seq++
	return defaultsID(1000 + w.seq)
}

func (w *defaultsWorld) initNode(bins string, withSecondHome bool) error {
	if err := w.setupNodeEnv(bins); err != nil {
		return err
	}
	w.ownerKey, w.hostKey = defaultsSeedKey(1), defaultsSeedKey(2)
	h1Key, h2Key := defaultsSeedKey(3), defaultsSeedKey(4)
	w.homeKeys = []identity.Key{h1Key, h2Key}
	allKeys := []identity.Key{w.ownerKey, w.hostKey, h1Key, h2Key}
	w.nowMS = 1_763_600_000_000
	clock := func() time.Time { return time.UnixMilli(w.nowMS) }
	if err := w.setupHomes(h1Key, h2Key, allKeys, withSecondHome, clock); err != nil {
		return err
	}
	if err := w.setupHost(clock); err != nil {
		return err
	}
	return w.declareInitialAccounts(bins)
}

func (w *defaultsWorld) setupNodeEnv(bins string) error {
	dir, err := hostAgentsSetBins(w.f.fakeCLIs, bins)
	if err != nil {
		return err
	}
	w.cleanup(func() { os.RemoveAll(dir) })
	prev, had := os.LookupEnv("PATH")
	if err := os.Setenv("PATH", hostAgentsIsolatedPath(dir)); err != nil {
		return err
	}
	w.cleanup(func() { restoreVar("PATH", prev, had) })
	rootRaw, err := os.MkdirTemp("", "solodawn-host-def-*")
	if err != nil {
		return err
	}
	w.cleanup(func() { os.RemoveAll(rootRaw) })
	w.root, err = filepath.EvalSymlinks(rootRaw)
	return err
}

func (w *defaultsWorld) setupHomes(h1Key, h2Key identity.Key, allKeys []identity.Key, withSecond bool, clock func() time.Time) error {
	w.homes = map[string]*defaultsHome{}
	w.groups = map[string]proto.ID{}
	w.grpHomes = map[string]*defaultsHome{}
	w.members = map[string]string{}
	keys := []identity.Key{h1Key}
	if withSecond {
		keys = append(keys, h2Key)
	}
	for _, hk := range keys {
		hn, err := newDefaultsHome(w.cleanup, w.ownerKey, hk, allKeys, w.hostKey.ID(), clock)
		if err != nil {
			return err
		}
		w.homes[hk.ID()] = hn
	}
	return nil
}

func (w *defaultsWorld) setupHost(clock func() time.Time) error {
	db, err := store.Open(filepath.Join(w.root, "node.db"))
	if err != nil {
		return err
	}
	w.cleanup(func() { db.Close() })
	w.db = db
	h, _, err := host.New(db, w.hostKey, clock)
	if err != nil {
		return err
	}
	w.cleanup(h.Settle)
	w.cleanup(w.disconnectAllHomes)
	w.h = h
	logs, err := node.OpenLogs(db, w.hostKey.ID())
	if err != nil {
		return err
	}
	w.sessions = node.NewSessions(node.SessionConfig{Store: db, Host: h, Logs: logs, ID: w.hostKey.ID(), Now: clock})
	node.NewSurface(node.SurfaceConfig{ID: w.hostKey.ID(), OS: "test", Version: "test", Store: db, Host: h, Sessions: w.sessions, Agents: h.Agents})
	h.Connect = w.connectHome
	return nil
}

func (w *defaultsWorld) disconnectAllHomes() {
	for _, hn := range w.homes {
		hn.disconnect()
	}
}

func (w *defaultsWorld) connectHome(_ context.Context, device string) {
	if hn := w.homes[device]; hn != nil {
		hn.attachHost(w.h)
	}
}

func (w *defaultsWorld) declareInitialAccounts(bins string) error {
	if _, err := w.h.Agents(context.Background(), false); err != nil {
		return err
	}
	for _, raw := range strings.Split(bins, ",") {
		if agent := strings.TrimSpace(raw); agent != "" {
			if err := w.declareAgentAccount(agent, "personal"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *defaultsWorld) ensureStarted(agent string, withSecondHome bool) error {
	if w.h != nil {
		return nil
	}
	return w.initNode(agent, withSecondHome)
}

func (w *defaultsWorld) declareAgentAccount(agent, account string) error {
	raw, _ := json.Marshal(proto.AgentSet{
		HostCommand: proto.HostCommand{Type: "agent.set", Nonce: w.nextID(), Timestamp: w.nowMS, To: w.hostKey.ID()},
		Agent:       agent,
		Account:     account,
	})
	_, err := host.Surface["agent.set"](context.Background(), w.h, nil, w.ownerKey.ID(), raw)
	return err
}

func (w *defaultsWorld) sendAgentDefaults(agent, model, effort, tier, skills string) error {
	raw, _ := json.Marshal(proto.AgentDefaults{
		HostCommand: proto.HostCommand{Type: "agent.defaults", Nonce: w.nextID(), Timestamp: w.nowMS, To: w.hostKey.ID()},
		Agent:       agent,
		Profile:     proto.Profile{Model: model, Effort: effort, Tier: tier, Skills: skills},
	})
	_, err := host.Surface["agent.defaults"](context.Background(), w.h, nil, w.ownerKey.ID(), raw)
	if err == nil {
		w.wantDefaults += w.countAgentGroups(agent)
	}
	return err
}

func (w *defaultsWorld) countAgentGroups(agent string) int {
	seen := map[string]bool{}
	for _, s := range w.h.Sessions.All() {
		if s.Task == "" && s.Agent == agent {
			seen[s.Group] = true
		}
	}
	return len(seen)
}

func (w *defaultsWorld) checkHostInfoDefaults(agent string, want *proto.Profile) error {
	reqRaw, _ := json.Marshal(proto.HostInfoRequest{Request: proto.Request{Type: "host.info", ID: 1}})
	ans, err := w.sessions.Requests["host.info"].Run(context.Background(), nil, nil, "", 1, reqRaw)
	if err != nil {
		return err
	}
	for _, entry := range ans.(proto.HostState).Agents {
		if entry.Agent == agent {
			return checkAgentDefaultsEntry(entry, want)
		}
	}
	return fmt.Errorf("host.info missing agent %s", agent)
}

func checkAgentDefaultsEntry(entry proto.HostAgent, want *proto.Profile) error {
	if want != nil {
		if entry.Defaults == nil || *entry.Defaults != *want {
			return fmt.Errorf("agent %s defaults = %+v, want %+v", entry.Agent, entry.Defaults, want)
		}
		return nil
	}
	if entry.Defaults != nil {
		return fmt.Errorf("agent %s has defaults %+v, want nil", entry.Agent, entry.Defaults)
	}
	raw, _ := json.Marshal(entry)
	if strings.Contains(string(raw), `"defaults"`) {
		return fmt.Errorf("agent %s JSON unexpectedly includes defaults: %s", entry.Agent, raw)
	}
	return nil
}

func (w *defaultsWorld) checkDefaultsBadAgent(agent, code string) error {
	err := w.sendAgentDefaults(agent, "", "", "standard", "")
	return expectHostError(err, code)
}

func (w *defaultsWorld) checkDefaultsBadTier(tier, code string) error {
	err := w.sendAgentDefaults("claude", "", "", tier, "")
	return expectHostError(err, code)
}

func (w *defaultsWorld) checkDefaultsMissingSkills(code string) error {
	raw := fmt.Sprintf(`{"t":"agent.defaults","n":%q,"ts":%d,"to":%q,"agent":"claude","model":"","effort":"","tier":"standard"}`,
		w.nextID(), w.nowMS, w.hostKey.ID())
	_, err := host.Surface["agent.defaults"](context.Background(), w.h, nil, w.ownerKey.ID(), json.RawMessage(raw))
	return expectHostError(err, code)
}

func expectHostError(err error, code string) error {
	var herr *host.Error
	if !errors.As(err, &herr) || herr.Code != code {
		return fmt.Errorf("got err %v, want *host.Error code %q", err, code)
	}
	return nil
}

func (w *defaultsWorld) givenClaudeDefaults(m, e, t, s string) error {
	if err := w.ensureStarted("claude,codex", false); err != nil {
		return err
	}
	return w.sendAgentDefaults("claude", m, e, t, s)
}

func (w *defaultsWorld) ensureGroup(name string, homeIdx int) error {
	if _, ok := w.groups[name]; ok {
		return nil
	}
	hn := w.homes[w.homeKeys[homeIdx].ID()]
	gid := w.nextID()
	_, err := hn.sendOwnerCmd(proto.GroupCreate{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: gid, Timestamp: w.nowMS, Group: gid},
			Op:          "create",
		},
		Name:       name,
		MemberName: "Owner",
	})
	if err != nil {
		return err
	}
	w.groups[name] = gid
	w.grpHomes[name] = hn
	return nil
}

func (w *defaultsWorld) joinGroup(agent, group, name string, prof proto.ProfileSet, follow bool) error {
	if err := w.ensureGroup(group, 0); err != nil {
		return err
	}
	gid := w.groups[group]
	hn := w.grpHomes[group]
	ws := filepath.Join(w.root, "work", group+"-"+name)
	if err := os.MkdirAll(ws, 0o700); err != nil {
		return err
	}
	payload := proto.HostJoin{
		HostCommand: proto.HostCommand{Type: "host.join", Nonce: w.nextID(), Timestamp: w.nowMS, To: w.hostKey.ID()},
		Group:       gid, Home: hn.key.ID(), Agent: agent, Name: name, Workspace: ws, ProfileSet: prof, Follow: proto.True(follow),
	}
	raw, _ := json.Marshal(payload)
	if _, err := host.Surface["host.join"](context.Background(), w.h, nil, w.ownerKey.ID(), raw); err != nil {
		return err
	}
	return w.recordJoinedMember(hn, group, string(gid), name)
}

func (w *defaultsWorld) recordJoinedMember(hn *defaultsHome, group, gid, name string) error {
	rows, err := hn.memberRows(gid)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.Name == name {
			w.members[group+":"+name] = r.ID
			break
		}
	}
	return w.waitCachedMember(gid, w.members[group+":"+name])
}

func (w *defaultsWorld) waitCachedMember(groupID, memberID string) error {
	return waitUntil(5*time.Second, fmt.Sprintf("host cached member %s", memberID), func() error {
		if _, ok := w.h.Rows.Get(groupID, "member", memberID); !ok {
			return errors.New("member row absent")
		}
		return nil
	})
}

func memberMatchesTarget(r proto.AIMemberRow, name string) bool {
	return r.Name == name || (name == "claude" && r.CLI == "claude" && !r.Secretary)
}

func (w *defaultsWorld) checkHomeMember(group, name string, wantFollow bool, want proto.Profile) error {
	gid := string(w.groups[group])
	rows, err := w.grpHomes[group].memberRows(gid)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if !memberMatchesTarget(r, name) {
			continue
		}
		if bool(r.Follow) != wantFollow || r.Profile != want {
			return fmt.Errorf("%s/%s: got follow=%v profile=%+v, want follow=%v profile=%+v", group, name, r.Follow, r.Profile, wantFollow, want)
		}
		return nil
	}
	return fmt.Errorf("member %q not found in %s (%d ai rows)", name, group, len(rows))
}

func (w *defaultsWorld) waitHomeMember(group, name string, wantFollow bool, want proto.Profile) error {
	gid := string(w.groups[group])
	hn := w.grpHomes[group]
	check := func() error { return w.checkMemberSynced(hn, gid, name, wantFollow, want) }
	return waitUntil(5*time.Second, fmt.Sprintf("%s/%s profile=%+v", group, name, want), check)
}

func (w *defaultsWorld) checkMemberSynced(hn *defaultsHome, gid, name string, wantFollow bool, want proto.Profile) error {
	rows, err := hn.memberRows(gid)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if !memberMatchesTarget(r, name) {
			continue
		}
		if rowMatchesProfile(r, wantFollow, want) && w.hostCacheMatches(gid, r.ID, wantFollow, want) {
			return nil
		}
		return fmt.Errorf("follow=%v profile=%+v", r.Follow, r.Profile)
	}
	return fmt.Errorf("%d rows", len(rows))
}

func rowMatchesProfile(r proto.AIMemberRow, wantFollow bool, want proto.Profile) bool {
	return bool(r.Follow) == wantFollow && r.Profile == want
}

func (w *defaultsWorld) hostCacheMatches(gid, id string, wantFollow bool, want proto.Profile) bool {
	raw, ok := w.h.Rows.Get(gid, "member", id)
	if !ok {
		return false
	}
	var cached proto.AIMemberRow
	return proto.Decode(raw, &cached) == nil && cached.Profile == want && bool(cached.Follow) == wantFollow
}

func (w *defaultsWorld) givenTwoHomeFollowers(g1, g2 string) error {
	if err := w.initNode("claude,codex", true); err != nil {
		return err
	}
	if err := w.ensureGroup(g1, 0); err != nil {
		return err
	}
	if err := w.ensureGroup(g2, 1); err != nil {
		return err
	}
	if err := w.joinGroup("claude", g1, "claude", proto.ProfileSet{}, true); err != nil {
		return err
	}
	return w.joinGroup("claude", g2, "claude", proto.ProfileSet{}, true)
}

func (w *defaultsWorld) givenOwnClaudeMember(group, m, e, t, s string) error {
	if w.h == nil {
		if err := w.ensureStarted("claude", false); err != nil {
			return err
		}
	}
	return w.joinGroup("claude", group, "claude-own", proto.ProfileSet{Model: &m, Effort: &e, Tier: &t, Skills: &s}, false)
}

func (w *defaultsWorld) givenSingleAppFollower(group string) error {
	if err := w.initNode("claude", false); err != nil {
		return err
	}
	return w.joinGroup("claude", group, "claude", proto.ProfileSet{}, true)
}

func (w *defaultsWorld) disconnectHome(group string) error {
	hn := w.grpHomes[group]
	hn.disconnect()
	return waitUntil(5*time.Second, "home disconnected", func() error {
		if _, ok := w.h.Home(hn.key.ID()); ok {
			return errors.New("still attached")
		}
		return nil
	})
}

func (w *defaultsWorld) reconnectHome(group string) error {
	hn := w.grpHomes[group]
	hn.reconnect(w.h)
	return nil
}

func (w *defaultsWorld) ownerSetsFollowOnDocs() error {
	gid := w.groups["Docs"]
	id := w.members["Docs:claude-own"]
	hn := w.grpHomes["Docs"]
	_, err := hn.sendOwnerCmd(proto.MemberSet{
		MemberCommand: proto.MemberCommand{
			HomeCommand: proto.HomeCommand{Type: "member", Nonce: w.nextID(), Timestamp: w.nowMS, Group: gid},
			Op:          "set",
			ID:          proto.ID(id),
		},
		Follow: true,
	})
	return err
}

func (w *defaultsWorld) checkSingleHostRestore(m, e, t, s string) error {
	want := proto.Profile{Model: m, Effort: e, Tier: t, Skills: s}
	if err := w.waitHomeMember("Docs", "claude-own", true, want); err != nil {
		return err
	}
	if err := w.settleWork(); err != nil {
		return err
	}
	id := w.members["Docs:claude-own"]
	sets := w.grpHomes["Docs"].hostSetsFor(id)
	if len(sets) != 1 || !memberSetMatches(sets[0], want) {
		return fmt.Errorf("got %d host member sets for %s (%+v), want 1 matching %+v", len(sets), id, sets, want)
	}
	return nil
}

func (w *defaultsWorld) settleWork() error {
	if err := waitUntil(5*time.Second, "defaults work settled", w.workQuiescent); err != nil {
		return err
	}
	w.disconnectAllHomes()
	w.h.Settle()
	return nil
}

func (w *defaultsWorld) workQuiescent() error {
	for _, hn := range w.homes {
		if hn.activePushes() > 0 {
			return errors.New("home push active")
		}
	}
	return w.diagBgSettled()
}

func (w *defaultsWorld) diagBgSettled() error {
	tag := "node=" + w.hostKey.ID()[:8]
	var starts, stops, defaultStops, groupStops, groupTriggers int
	for line := range strings.SplitSeq(diagLog.String(), "\n") {
		if !strings.Contains(line, tag) {
			continue
		}
		starts += countSub(line, "bg.start")
		stops += countSub(line, "bg.stop")
		defaultStops += countAllSub(line, "bg.stop", "name=defaults")
		groupStops += countAllSub(line, "bg.stop", "name=group_state")
		groupTriggers += countDiagTrigger(line)
	}
	return checkBgCounts(starts, stops, defaultStops, w.wantDefaults, groupStops, groupTriggers)
}

func countSub(line, sub string) int {
	if strings.Contains(line, sub) {
		return 1
	}
	return 0
}

func countAllSub(line, a, b string) int {
	if strings.Contains(line, a) && strings.Contains(line, b) {
		return 1
	}
	return 0
}

func countDiagTrigger(line string) int {
	if strings.Contains(line, "rows.recv") || (strings.Contains(line, "sync.end") && strings.Contains(line, "ok=true")) {
		return 1
	}
	return 0
}

func checkBgCounts(starts, stops, defStops, wantDef, grpStops, wantGrp int) error {
	if starts != stops {
		return fmt.Errorf("bg starts=%d stops=%d", starts, stops)
	}
	if defStops < wantDef {
		return fmt.Errorf("defaults stops=%d want>=%d", defStops, wantDef)
	}
	if grpStops < wantGrp {
		return fmt.Errorf("group_state stops=%d want>=%d", grpStops, wantGrp)
	}
	return nil
}

func memberSetMatches(got proto.MemberSet, want proto.Profile) bool {
	if !bool(got.Follow) {
		return false
	}
	for _, pair := range []struct {
		ptr  *string
		want string
	}{{got.Model, want.Model}, {got.Effort, want.Effort}, {got.Tier, want.Tier}, {got.Skills, want.Skills}} {
		if pair.ptr == nil || *pair.ptr != pair.want {
			return false
		}
	}
	return true
}
