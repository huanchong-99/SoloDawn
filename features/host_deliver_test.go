//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
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
	stepSets["host_deliver"] = hostDeliverSteps
}

const (
	accGroupID = "AAAAAAAAAAAAAAAAAAAAAQ"
	accMember1 = "BBBBBBBBBBBBBBBBBBBBBQ"
	accSecID   = "CCCCCCCCCCCCCCCCCCCCCQ"
	accTaskID  = "DDDDDDDDDDDDDDDDDDDDDQ"
	accCallID  = "EEEEEEEEEEEEEEEEEEEEEQ"
	accNowMS   = int64(1763600000000)
)

type accSyncReply struct {
	raw json.RawMessage
	err error
}

type accHome struct {
	mu       sync.Mutex
	done     chan struct{}
	acks     []proto.Ack
	sinces   []int64
	started  chan struct{} // single reader: attachNoWait
	finished chan struct{} // single reader: attachAndFinishSync / sync steps
	reply    chan accSyncReply
}

func newAccHome() *accHome {
	return &accHome{
		done:     make(chan struct{}),
		started:  make(chan struct{}, 4),
		finished: make(chan struct{}, 4),
		reply:    make(chan accSyncReply, 4),
	}
}

func (f *accHome) Send(frame any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ack, ok := frame.(proto.Ack); ok {
		f.acks = append(f.acks, ack)
	}
	return nil
}

func (f *accHome) Path() string                 { return "local" }
func (f *accHome) PathChanged() <-chan struct{} { return nil }
func (f *accHome) Done() <-chan struct{}        { return f.done }

func (f *accHome) Request(ctx context.Context, build func(int64) any) (json.RawMessage, error) {
	defer func() { f.finished <- struct{}{} }()
	req := build(1)
	f.recordSync(req)
	f.started <- struct{}{}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case ans := <-f.reply:
		return ans.raw, ans.err
	}
}

func (f *accHome) recordSync(req any) {
	if s, ok := req.(proto.Sync); ok {
		f.mu.Lock()
		f.sinces = append(f.sinces, s.Since)
		f.mu.Unlock()
	}
}

func (f *accHome) ackCount(item int64) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, a := range f.acks {
		if a.Item == item {
			n++
		}
	}
	return n
}

func (f *accHome) allAcks() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.acks)
}

type accRunner struct {
	mu    sync.Mutex
	turns map[string]int
}

func (r *accRunner) Turn(_ context.Context, h *host.Host, key host.SessionKey, _ string, _ host.Turn) error {
	if s, ok := h.Sessions.Get(key); ok && s.State == host.Dormant {
		h.Sessions.Move(key, host.Dormant, host.Working)
		h.Sessions.Move(key, host.Working, host.Waiting)
	}
	r.mu.Lock()
	r.turns[key.Member]++
	r.mu.Unlock()
	return nil
}

func (r *accRunner) Answer(*host.Host, host.SessionKey, string, bool) error { return nil }

func (r *accRunner) count(member string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.turns[member]
}

type accDeliverEnv struct {
	dir     string
	db      *store.Store
	h       *host.Host
	hostKey identity.Key
	homeKey identity.Key
	home    *accHome
	sinces  []int64
	runner  *accRunner
	askCh   <-chan json.RawMessage
	askDone func()
}

func makeAccKey(seed byte) identity.Key {
	buf := bytes.Repeat([]byte{seed}, ed25519.SeedSize)
	return identity.Key(ed25519.NewKeyFromSeed(buf))
}

func makeAccID(tag byte) proto.ID {
	var raw [16]byte
	raw[0] = tag
	return proto.ID(base64.RawURLEncoding.EncodeToString(raw[:]))
}

func (e *accDeliverEnv) setupKeysAndStore(dir string) error {
	e.dir = dir
	e.hostKey = makeAccKey(2)
	e.homeKey = makeAccKey(3)
	e.runner = &accRunner{turns: map[string]int{}}
	db, err := store.Open(filepath.Join(dir, "node.db"))
	if err != nil {
		return err
	}
	e.db = db
	if err := e.seedHomes(); err != nil {
		return err
	}
	return e.startHost()
}

func (e *accDeliverEnv) seedHomes() error {
	homes := accJSON([]proto.Home{{Group: accGroupID, Home: e.homeKey.ID(), Name: "Website"}})
	write := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('homes', ?)", homes)
		return err
	}
	return e.db.Write(write)
}

func (e *accDeliverEnv) startHost() error {
	clock := func() time.Time { return time.UnixMilli(accNowMS) }
	h, _, err := host.New(e.db, e.hostKey, clock)
	e.h = h
	return err
}

func (e *accDeliverEnv) closeHome() {
	if e.home == nil {
		return
	}
	select {
	case <-e.home.done:
	default:
		close(e.home.done)
	}
}

func (e *accDeliverEnv) seedLiveGroup() {
	e.h.Rows.Put(accGroupID, "group", accGroupID, json.RawMessage(accGroupRowJSON(e.homeKey.ID(), 1)))
	e.h.Rows.Put(accGroupID, "member", accMember1, json.RawMessage(accMemberRowJSON(accMember1, "Claude Code", e.hostKey.ID(), false, 2)))
	e.h.Rows.Put(accGroupID, "member", accSecID, json.RawMessage(accMemberRowJSON(accSecID, "Sec", e.hostKey.ID(), true, 3)))
	e.attachAndFinishSync(3)
}

func (e *accDeliverEnv) attachNoWait() {
	e.closeHome()
	e.home = newAccHome()
	e.h.Attach(e.homeKey.ID(), e.home)
	<-e.home.started
}

func (e *accDeliverEnv) attachAndFinishSync(seq int64) {
	e.attachNoWait()
	ans := accJSON(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: accGroupID, Seq: seq})
	e.home.reply <- accSyncReply{raw: json.RawMessage(ans)}
	<-e.home.finished
	e.sinces = append(e.sinces, e.home.sinces...)
}

func (e *accDeliverEnv) waitCondition(desc string, ok func() bool) error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return nil
		}
		time.Sleep(2 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", desc)
}

func accGroupRowJSON(homeID string, seq int64) string {
	return accJSON(proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: accGroupID, Seq: seq},
		Name:      "Website", Kind: "project", Base: "main", Forge: "none",
		Home: homeID, Secretary: accSecID, Checks: []string{},
	})
}

func accMemberRowJSON(id, name, hostID string, sec bool, seq int64) string {
	return accJSON(proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id, Seq: seq}, Group: accGroupID, Kind: "ai", User: "u", Name: name},
		CLI:       "claude", Host: hostID, HostName: "host", Secretary: sec, Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local",
	})
}

func accTaskRowJSON(state string, seq int64) string {
	return accJSON(proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: accTaskID, Seq: seq},
		Group:     accGroupID, Num: 1, Title: "Fix", Slug: "fix", Origin: "brief",
		State: state, Step: "branch", Assignee: accMember1, Depends: []string{},
	})
}

func accJSON(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return strings.TrimSuffix(b.String(), "\n")
}

func (e *accDeliverEnv) pushSignedItem(signer identity.Key, connDev string, it proto.Item, msg *proto.MsgRow) {
	wire := proto.InboxItem{Env: signer.Sign(accJSON(it))}
	if msg != nil {
		wire.Msg = json.RawMessage(accJSON(msg))
	}
	frame := accJSON(proto.Inbox{Type: "inbox", Group: accGroupID, Items: []proto.InboxItem{wire}})
	e.h.Push(connDev, json.RawMessage(frame))
}

func hostDeliverSteps(sc *godog.ScenarioContext, f fixtures) {
	cleanup := afterScenario(sc)
	prevRunner, prevCompose := host.Runner, host.Compose
	env := &accDeliverEnv{}
	installHooks := func() {
		host.Runner = env.runner
		host.Compose = func(_ *host.Host, _ host.Session, kind host.TurnKind, items []json.RawMessage) (string, host.Turn, error) {
			return fmt.Sprintf("items=%d", len(items)), host.Turn{Kind: kind}, nil
		}
		cleanup(func() {
			host.Runner = prevRunner
			host.Compose = prevCompose
		})
	}
	registerDeliverGivenSteps(sc, f, cleanup, env, installHooks)
	registerDeliverWhenSteps(sc, env)
	registerDeliverThenSteps(sc, env)
}

func registerDeliverGivenSteps(sc *godog.ScenarioContext, f fixtures, cleanup func(func()), env *accDeliverEnv, installHooks func()) {
	initStore := func() error {
		if err := env.setupKeysAndStore(f.t.TempDir()); err != nil {
			return err
		}
		installHooks()
		cleanup(func() {
			env.closeHome()
			if env.db != nil {
				env.db.Close()
			}
		})
		return nil
	}
	sc.Step(`^a paired host on disk with AI "Claude Code" in group "Website" homed on "home-1"$`, func() error {
		if err := initStore(); err != nil {
			return err
		}
		env.seedLiveGroup()
		return nil
	})
	sc.Step(`^a paired host on disk configured for group "Website" on "home-1"$`, initStore)
	sc.Step(`^a paired host on disk with AI "Claude Code" working on a turn in group "Website" on "home-1"$`, func() error {
		if err := initStore(); err != nil {
			return err
		}
		env.seedLiveGroup()
		key := host.SessionKey{Group: accGroupID, Member: accMember1}
		s := host.Session{SessionKey: key, Kind: host.GroupSession, Folder: "repo", Agent: "claude", Home: env.homeKey.ID(), Name: "Claude Code"}
		if err := env.h.Sessions.Create(s); err != nil {
			return err
		}
		return env.h.Sessions.Move(key, host.Dormant, host.Working)
	})
	sc.Step(`^"Claude Code" has an open ask waiting on call "call-1"$`, func() {
		env.askCh, env.askDone = env.h.Await(accCallID, nil)
	})
}

func registerDeliverWhenSteps(sc *godog.ScenarioContext, env *accDeliverEnv) {
	registerBasicPushSteps(sc, env)
	registerSyncFlowSteps(sc, env)
}

func registerBasicPushSteps(sc *godog.ScenarioContext, env *accDeliverEnv) {
	sc.Step(`^"home-1" pushes mention item (\d+) for "Claude Code" twice$`, func(num int) error {
		env.pushMention(int64(num))
		firstAck := func() bool { return env.home.ackCount(int64(num)) == 1 }
		if err := env.waitCondition("first mention delivery", firstAck); err != nil {
			return err
		}
		env.pushMention(int64(num))
		return nil
	})
	sc.Step(`^the host restarts on its store and "home-1" pushes mention item (\d+) again$`, func(num int) error {
		prevAcks := env.home.acks
		if err := env.startHost(); err != nil {
			return err
		}
		env.seedLiveGroup()
		env.home.mu.Lock()
		env.home.acks = append(prevAcks, env.home.acks...)
		env.home.mu.Unlock()
		env.pushMention(int64(num))
		return nil
	})
	sc.Step(`^"home-1" pushes reply item (\d+) answering "call-1" and mention item (\d+) for "Claude Code"$`, func(rNum, mNum int) {
		replyID := makeAccID(31)
		callRef := accCallID
		msg := &proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: string(replyID), Seq: 20},
			Pos:       20, Group: accGroupID, Main: false, By: accSecID, Kind: "reply", Timestamp: accNowMS,
			Text: "answer", Mentions: []string{}, Re: &callRef, Body: json.RawMessage(`{}`),
		}
		it := proto.Item{Type: "item", Nonce: makeAccID(32), Timestamp: accNowMS, Group: accGroupID, To: env.hostKey.ID(), Item: int64(rNum), AI: accMember1, Why: "reply", Msg: replyID}
		env.pushSignedItem(env.homeKey, env.homeKey.ID(), it, msg)
		env.pushMention(int64(mNum))
	})
	sc.Step(`^the running turn of "Claude Code" finishes and calls Deliver$`, func() error {
		if env.askDone != nil {
			env.askDone()
		}
		key := host.SessionKey{Group: accGroupID, Member: accMember1}
		if err := env.h.Sessions.Move(key, host.Working, host.Waiting); err != nil {
			return err
		}
		host.Deliver(env.h, accGroupID, accMember1)
		return nil
	})
}

func (e *accDeliverEnv) pushMention(itemNum int64) {
	msgID := makeAccID(byte(40 + itemNum))
	msg := &proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(msgID), Seq: 10 + itemNum},
		Pos:       10 + itemNum, Group: accGroupID, Main: true, By: accSecID, Kind: "text", Timestamp: accNowMS,
		Text: "hi", Mentions: []string{accMember1}, Body: json.RawMessage(`{}`),
	}
	it := proto.Item{Type: "item", Nonce: makeAccID(byte(80 + itemNum)), Timestamp: accNowMS, Group: accGroupID, To: e.hostKey.ID(), Item: itemNum, AI: accMember1, Why: "mention", Msg: msgID}
	e.pushSignedItem(e.homeKey, e.homeKey.ID(), it, msg)
}

func registerSyncFlowSteps(sc *godog.ScenarioContext, env *accDeliverEnv) {
	sc.Step(`^the host attaches to "home-1" while sync is in flight$`, func() {
		env.attachNoWait()
	})
	sc.Step(`^"home-1" pushes assign item (\d+) for "Claude Code" on task "T-1" and notice item (\d+) for secretary "Sec"$`, func(aNum, nNum int) {
		aIt := proto.Item{Type: "item", Nonce: makeAccID(21), Timestamp: accNowMS, Group: accGroupID, To: env.hostKey.ID(), Item: int64(aNum), AI: accMember1, Why: "assign", Task: accTaskID}
		nIt := proto.Item{Type: "item", Nonce: makeAccID(22), Timestamp: accNowMS, Group: accGroupID, To: env.hostKey.ID(), Item: int64(nNum), AI: accSecID, Why: "notice", Task: accTaskID, Code: "task_merged"}
		env.pushSignedItem(env.homeKey, env.homeKey.ID(), aIt, nil)
		env.pushSignedItem(env.homeKey, env.homeKey.ID(), nIt, nil)
	})
	sc.Step(`^"home-1" completes sync at sequence (\d+) with task "T-1" already merged$`, func(seq int) error {
		rows := []json.RawMessage{
			json.RawMessage(accGroupRowJSON(env.homeKey.ID(), 1)),
			json.RawMessage(accMemberRowJSON(accMember1, "Claude Code", env.hostKey.ID(), false, 2)),
			json.RawMessage(accMemberRowJSON(accSecID, "Sec", env.hostKey.ID(), true, 3)),
			json.RawMessage(accTaskRowJSON("merged", int64(seq))),
		}
		env.h.Push(env.homeKey.ID(), json.RawMessage(accJSON(proto.Rows{Type: "rows", Group: accGroupID, Rows: rows})))
		ans := accJSON(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: accGroupID, Seq: int64(seq)})
		env.home.reply <- accSyncReply{raw: json.RawMessage(ans)}
		<-env.home.finished
		return env.waitCondition("2 acks and secretary turn", func() bool {
			return env.home.allAcks() == 2 && env.runner.count(accSecID) == 1
		})
	})
	sc.Step(`^the host attaches to "home-1" and an initial reset breaks off after row sequence (\d+)$`, func(seq int) error {
		env.attachNoWait()
		env.h.Push(env.homeKey.ID(), json.RawMessage(accJSON(proto.Reset{Type: "reset", Group: accGroupID})))
		rows := []json.RawMessage{json.RawMessage(accGroupRowJSON(env.homeKey.ID(), int64(seq)))}
		env.h.Push(env.homeKey.ID(), json.RawMessage(accJSON(proto.Rows{Type: "rows", Group: accGroupID, Rows: rows})))
		env.home.reply <- accSyncReply{err: errors.New("eof")}
		<-env.home.finished
		gotSince := func() bool { return len(env.home.sinces) == 1 }
		if err := env.waitCondition("initial sync request", gotSince); err != nil {
			return err
		}
		env.sinces = append(env.sinces, env.home.sinces...)
		return nil
	})
	sc.Step(`^the host reconnects to "home-1" and completes sync at sequence (\d+)$`, func(seq int) {
		env.attachAndFinishSync(int64(seq))
	})
	sc.Step(`^"home-1" pushes live row sequence (\d+) and a reconnect delta breaks off before synced$`, func(seq int) {
		rows := []json.RawMessage{json.RawMessage(accMemberRowJSON(accMember1, "Claude Code", env.hostKey.ID(), false, int64(seq)))}
		env.h.Push(env.homeKey.ID(), json.RawMessage(accJSON(proto.Rows{Type: "rows", Group: accGroupID, Rows: rows})))
		env.attachNoWait()
		rows2 := []json.RawMessage{json.RawMessage(accMemberRowJSON(accSecID, "Sec", env.hostKey.ID(), true, int64(seq+1)))}
		env.h.Push(env.homeKey.ID(), json.RawMessage(accJSON(proto.Rows{Type: "rows", Group: accGroupID, Rows: rows2})))
		env.home.reply <- accSyncReply{err: errors.New("eof")}
		<-env.home.finished
		env.sinces = append(env.sinces, env.home.sinces...)
	})
}

func registerDeliverThenSteps(sc *godog.ScenarioContext, env *accDeliverEnv) {
	sc.Step(`^the delivery audit records (\d+) "refused" row and (\d+) "delivered" rows?$`, func(wantRef, wantDel int) error {
		var gotRef, gotDel int
		read := func(tx *store.Tx) error {
			tx.QueryRow("SELECT count(*) FROM audit WHERE what = 'refused'").Scan(&gotRef)
			return tx.QueryRow("SELECT count(*) FROM audit WHERE what = 'delivered'").Scan(&gotDel)
		}
		if err := env.db.Write(read); err != nil {
			return err
		}
		if gotRef != wantRef || gotDel != wantDel {
			return fmt.Errorf("audit refused=%d delivered=%d, want refused=%d delivered=%d", gotRef, gotDel, wantRef, wantDel)
		}
		return nil
	})
	sc.Step(`^"Claude Code" has received (\d+) turns? and "home-1" has received (\d+) acknowledgements? for item (\d+)$`, func(wantTurns, wantAcks, item int) error {
		err := env.waitCondition("turns and acknowledgements", func() bool {
			return env.runner.count(accMember1) == wantTurns && env.home.ackCount(int64(item)) == wantAcks
		})
		if err != nil {
			return fmt.Errorf("%w: turns=%d acks=%d, want turns=%d acks=%d", err, env.runner.count(accMember1), env.home.ackCount(int64(item)), wantTurns, wantAcks)
		}
		return nil
	})
	sc.Step(`^"Claude Code" has received (\d+) turns? and "home-1" has received 0 acknowledgements$`, func(wantTurns int) error {
		if env.runner.count(accMember1) != wantTurns || env.home.allAcks() != 0 {
			return fmt.Errorf("turns=%d acks=%d", env.runner.count(accMember1), env.home.allAcks())
		}
		return nil
	})
	sc.Step(`^"Sec" has received (\d+) turn and "Claude Code" has received (\d+) turns$`, func(wantSec, wantAI int) error {
		secReady := func() bool { return env.runner.count(accSecID) == wantSec }
		if err := env.waitCondition("secretary turn", secReady); err != nil {
			return err
		}
		if env.runner.count(accSecID) != wantSec || env.runner.count(accMember1) != wantAI {
			return fmt.Errorf("sec=%d ai=%d, want sec=%d ai=%d", env.runner.count(accSecID), env.runner.count(accMember1), wantSec, wantAI)
		}
		return nil
	})
	sc.Step(`^the sync requests asked for since "([^"]+)"$`, func(want string) error {
		parts := make([]string, len(env.sinces))
		for i, v := range env.sinces {
			parts[i] = strconv.FormatInt(v, 10)
		}
		if got := strings.Join(parts, ","); got != want {
			return fmt.Errorf("got since %q, want %q", got, want)
		}
		return nil
	})
	sc.Step(`^the open ask receives item (\d+) and "Claude Code" has received (\d+) new turns$`, func(wantItem, wantTurns int) error {
		gotReply := func() bool { return len(env.askCh) == 1 }
		if err := env.waitCondition("open ask reply", gotReply); err != nil {
			return err
		}
		if env.runner.count(accMember1) != wantTurns {
			return fmt.Errorf("turns=%d, want %d", env.runner.count(accMember1), wantTurns)
		}
		raw := <-env.askCh
		var wire proto.InboxItem
		proto.Decode(raw, &wire)
		var it proto.Item
		proto.Decode([]byte(wire.Env.Payload), &it)
		if int(it.Item) != wantItem {
			return fmt.Errorf("got item %d, want %d", it.Item, wantItem)
		}
		return nil
	})
	sc.Step(`^"Claude Code" has received (\d+) new turn and "home-1" has received (\d+) acknowledgement for item (\d+)$`, func(wantTurns, wantAcks, item int) error {
		return env.waitCondition("post-turn delivery", func() bool {
			return env.runner.count(accMember1) == wantTurns && env.home.ackCount(int64(item)) == wantAcks
		})
	})
}
