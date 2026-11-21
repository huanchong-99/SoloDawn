//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing/fstest"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["m1"] = func(sc *godog.ScenarioContext, _ fixtures) {
		m1Steps(sc)
	}
}

type m1Browser struct {
	key    identity.Key
	peer   *peer.Peer
	path   *peer.Path
	conn   *peer.Connection
	pushes <-chan json.RawMessage // single reader: nextPush in scenario steps
}

func (b *m1Browser) nextPush(ctx context.Context) (json.RawMessage, error) {
	select {
	case msg, ok := <-b.pushes:
		if !ok {
			return nil, errors.New("ctl closed before push")
		}
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type m1World struct {
	ctx        context.Context
	cancel     context.CancelFunc
	cleanup    func(func())
	now        time.Time
	rvURL      string
	db         *store.Store
	nodeKey    identity.Key
	logs       *node.Logs
	loop       *home.Loop
	host       *host.Host
	sessions   *node.Sessions
	pairer     *node.Pairer
	nodePeer   *peer.Peer
	nodeRVPath *peer.Path
	accepted   chan *peer.Connection // single reader: acceptInbound
	browsers   []*m1Browser
	createdGrp proto.ID
}

func m1Steps(sc *godog.ScenarioContext) {
	w := &m1World{
		cleanup: afterScenario(sc),
		now:     time.Now(),
	}
	sc.Step(`^an M1 owned node with two already-connected browsers of its owner$`, w.setupTwoConnectedBrowsers)
	sc.Step(`^the first M1 browser creates group "([^"]*)"$`, w.createGroupFromFirstBrowser)
	sc.Step(`^the second M1 browser receives a groups push listing "([^"]*)" within 1 second without reconnecting$`, w.checkSecondBrowserGroupsPush)
	sc.Step(`^the second M1 browser syncs "([^"]*)" and the first M1 browser posts "([^"]*)"$`, w.syncSecondAndPostFirst)
	sc.Step(`^the second M1 browser receives that message row within 1 second$`, w.checkSecondBrowserMessageRow)
}

func (w *m1World) nowTime() time.Time { return w.now }

func (w *m1World) setupTwoConnectedBrowsers() error {
	if err := w.setupInProcessOwned(2); err != nil {
		return err
	}
	for _, b := range w.browsers {
		if err := w.connectAndHelloBrowser(b); err != nil {
			return err
		}
	}
	return nil
}

func (w *m1World) setupInProcessOwned(browserCount int) error {
	w.ctx, w.cancel = context.WithTimeout(context.Background(), 30*time.Second)
	w.cleanup(w.cancel)
	srv := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
	w.cleanup(srv.Close)
	w.rvURL = "ws" + strings.TrimPrefix(srv.URL, "http") + "/rv"
	w.nodeKey = exampleKey("m1-inproc-node")
	w.accepted = make(chan *peer.Connection, 8)
	for i := range browserCount {
		b, err := w.newM1Browser(fmt.Sprintf("browser-%d", i))
		if err != nil {
			return err
		}
		w.browsers = append(w.browsers, b)
	}
	return w.startInProcessNode()
}

func (w *m1World) newM1Browser(label string) (*m1Browser, error) {
	key := exampleKey("m1-" + label)
	socks, err := peer.Listen(0, 0)
	if err != nil {
		return nil, err
	}
	db, err := store.Open(":memory:")
	if err != nil {
		socks.Close()
		return nil, err
	}
	w.cleanup(func() { socks.Close(); db.Close() })
	rvPath, err := peer.Dial(w.ctx, w.rvURL, key)
	if err != nil {
		return nil, err
	}
	w.cleanup(func() { rvPath.Close() })
	b := &m1Browser{key: key, path: rvPath}
	known := func(id string) bool { return id == w.nodeKey.ID() }
	routes := func(string) []peer.Route { return []peer.Route{b.path.Route()} }
	b.peer = peer.New(key, socks, db, known, routes, func(*peer.Connection) {}, nil)
	go rvPath.Serve(w.ctx, b.peer, func(string, []byte) error { return nil })
	return b, nil
}

func (w *m1World) startInProcessNode() error {
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	w.cleanup(func() { db.Close() })
	w.db = db
	if err := w.seedInProcessOwner(); err != nil {
		return err
	}
	if err := w.initInProcessServices(); err != nil {
		return err
	}
	return w.startInProcessPeer()
}

func (w *m1World) seedInProcessOwner() error {
	owner := w.browsers[0].key
	now := w.now.UnixMilli()
	entries := []proto.Envelope{
		m1SignDeviceAdd(owner, owner.ID(), 1, owner.ID(), "browser", "b0", now),
		m1SignDeviceAdd(owner, owner.ID(), 2, w.nodeKey.ID(), "node", "m1-node", now),
	}
	for i := 1; i < len(w.browsers); i++ {
		entries = append(entries, m1SignDeviceAdd(owner, owner.ID(), len(entries)+1, w.browsers[i].key.ID(), "browser", fmt.Sprintf("b%d", i), now))
	}
	return w.writeSeededStore(owner.ID(), entries)
}

func (w *m1World) writeSeededStore(ownerID string, entries []proto.Envelope) error {
	ownerJSON, err := json.Marshal(ownerID)
	if err != nil {
		return err
	}
	rvJSON, err := json.Marshal([]string{w.rvURL})
	if err != nil {
		return err
	}
	write := func(tx *store.Tx) error {
		_, wErr := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', ?), ('name', '\"m1-node\"'), ('rendezvous', ?)", string(ownerJSON), string(rvJSON))
		return errors.Join(wErr, m1WriteLogEntries(tx, ownerID, entries))
	}
	return w.db.Write(write)
}

func m1WriteLogEntries(tx *store.Tx, ownerID string, entries []proto.Envelope) error {
	var allErr error
	for i, env := range entries {
		raw, err := json.Marshal(env)
		if err != nil {
			return err
		}
		_, wErr := tx.Exec("INSERT INTO device_log(user, ver, env) VALUES(?, ?, ?)", ownerID, i+1, string(raw))
		allErr = errors.Join(allErr, wErr)
	}
	return allErr
}

func (w *m1World) initInProcessServices() error {
	logs, err := node.OpenLogs(w.db, w.nodeKey.ID())
	if err != nil {
		return err
	}
	w.logs = logs
	loop, err := home.StartLoop(home.LoopConfig{
		Store: w.db, Home: w.nodeKey.ID(), Owner: logs.Owner(),
		Logs: logs.Log, Tables: home.NodeTables(), Now: w.nowTime,
	})
	if err != nil {
		return err
	}
	w.cleanup(loop.Close)
	w.loop = loop
	h, _, err := host.New(w.db, w.nodeKey, w.nowTime)
	if err != nil {
		return err
	}
	w.host = h
	link := node.AttachLocal(h, loop)
	w.cleanup(func() { link.Close() })
	w.wireInProcessSurfaceAndPairer()
	return nil
}

func (w *m1World) wireInProcessSurfaceAndPairer() {
	nameFn := func() string { return node.ReadName(w.db) }
	w.sessions = node.NewSessions(node.SessionConfig{
		ID: w.nodeKey.ID(), Version: "v0.1.0", Store: w.db, Logs: w.logs, Home: w.loop, Host: w.host,
		Name: nameFn, Now: w.nowTime,
	})
	node.NewSurface(node.SurfaceConfig{
		ID: w.nodeKey.ID(), Version: "v0.1.0", Store: w.db, Sessions: w.sessions,
		Host: w.host, Name: nameFn, Agents: w.host.Agents,
	})
	w.pairer = node.NewPairer(node.PairConfig{
		Key: w.nodeKey, Store: w.db, Logs: w.logs, Sessions: w.sessions,
		Paths: func() []*peer.Path { return []*peer.Path{w.nodeRVPath} },
		Now:   w.nowTime,
	})
	w.cleanup(w.pairer.Close)
}

func (w *m1World) startInProcessPeer() error {
	socks, err := peer.Listen(0, 0)
	if err != nil {
		return err
	}
	w.cleanup(func() { socks.Close() })
	rvPath, err := peer.Dial(w.ctx, w.rvURL, w.nodeKey)
	if err != nil {
		return err
	}
	w.cleanup(func() { rvPath.Close() })
	w.nodeRVPath = rvPath
	known := func(dev string) bool { return w.logs.Known(dev) || w.pairer.Known(dev) }
	routes := func(dev string) []peer.Route {
		return slices.Concat([]peer.Route{w.nodeRVPath.Route()}, w.pairer.Routes(dev))
	}
	inbound := func(c *peer.Connection) {
		w.accepted <- c
		go w.pairer.Serve(w.ctx, c.Device, c)
	}
	w.nodePeer = peer.New(w.nodeKey, socks, w.db, known, routes, inbound, nil)
	w.pairer.Peer = w.nodePeer
	event := func(kind string, data []byte) error {
		w.pairer.HandleEvent(w.ctx, rvPath, kind, data)
		return nil
	}
	go rvPath.Serve(w.ctx, w.nodePeer, event)
	return nil
}

func (w *m1World) connectAndHelloBrowser(b *m1Browser) error {
	conn, pushes, err := w.dialPeerCtl(b.peer, w.nodeKey.ID())
	if err != nil {
		return err
	}
	b.conn, b.pushes = conn, pushes
	served, err := w.acceptInbound()
	if err != nil {
		return err
	}
	if err := m1CheckDirectBoth(conn, served); err != nil {
		return err
	}
	if err := w.sendHello(conn); err != nil {
		return err
	}
	return m1DrainInitialDevlog(w.ctx, b)
}

func m1CheckDirectBoth(a, b *peer.Connection) error {
	if err := m1WaitPath(a, "direct", time.Second); err != nil {
		return err
	}
	return m1WaitPath(b, "direct", time.Second)
}

func m1WaitPath(conn *peer.Connection, want string, limit time.Duration) error {
	timer := time.NewTimer(limit)
	defer timer.Stop()
	for {
		if conn.Path() == want {
			return nil
		}
		if !m1WaitPathSignal(conn, timer.C) {
			return fmt.Errorf("path %q after %s, want %q", conn.Path(), limit, want)
		}
	}
}

func m1WaitPathSignal(conn *peer.Connection, timeout <-chan time.Time) bool {
	select {
	case <-conn.PathChanged():
		return true
	case <-conn.Done():
		return false
	case <-timeout:
		return false
	}
}

func (w *m1World) dialPeerCtl(p *peer.Peer, target string) (*peer.Connection, <-chan json.RawMessage, error) {
	conn, err := p.Offer(w.ctx, target)
	if err != nil {
		return nil, nil, err
	}
	w.cleanup(func() { conn.Close() })
	select {
	case <-w.ctx.Done():
		return nil, nil, w.ctx.Err()
	case <-conn.Done():
		return nil, nil, errors.New("ctl closed before opening")
	case <-conn.Ready():
		pushes := make(chan json.RawMessage, 64) // single reader: m1Browser.nextPush
		go m1PumpPushes(w.ctx, conn, pushes)
		return conn, pushes, nil
	}
}

func m1PumpPushes(ctx context.Context, conn *peer.Connection, pushes chan<- json.RawMessage) {
	defer close(pushes)
	for {
		msg, err := conn.Read(ctx)
		if err != nil {
			return
		}
		select {
		case pushes <- msg:
		case <-ctx.Done():
			return
		case <-conn.Done():
			return
		}
	}
}

func (w *m1World) acceptInbound() (*peer.Connection, error) {
	select {
	case <-w.ctx.Done():
		return nil, w.ctx.Err()
	case c := <-w.accepted:
		return c, nil
	}
}

func (w *m1World) sendHello(conn *peer.Connection) error {
	build := func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 1, Kind: "browser", Name: "browser", Addrs: []string{}}
	}
	raw, err := conn.Request(w.ctx, build)
	if err != nil {
		return err
	}
	var welcome proto.Welcome
	if err := proto.Decode(raw, &welcome); err != nil || !welcome.Owner {
		return fmt.Errorf("welcome = %s (%v)", raw, err)
	}
	return nil
}

func m1DrainInitialDevlog(ctx context.Context, b *m1Browser) error {
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	raw, err := b.nextPush(readCtx)
	if err != nil {
		return err
	}
	var f struct {
		Type string `json:"t"`
	}
	if proto.Decode(raw, &f) != nil || f.Type != "devlog" {
		return fmt.Errorf("expected initial devlog push, got %s", raw)
	}
	return nil
}

func (w *m1World) createGroupFromFirstBrowser(name string) error {
	b := w.browsers[0]
	gid := exampleID("m1-group-" + name)
	w.createdGrp = gid
	payload := fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q,"name":%q,"kind":"project","repo":"/tmp/website","base":"main","forge":"none","checks":[],"member_name":"Owner"}`,
		gid, w.now.UnixMilli(), gid, name)
	env := b.key.Sign(payload)
	raw, err := b.conn.Request(w.ctx, func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	})
	if err != nil {
		return err
	}
	var ok proto.Success
	return proto.Decode(raw, &ok)
}

func (w *m1World) checkSecondBrowserGroupsPush(wantName string) error {
	readCtx, cancel := context.WithTimeout(w.ctx, time.Second)
	defer cancel()
	for {
		raw, err := w.browsers[1].nextPush(readCtx)
		if err != nil {
			return fmt.Errorf("waiting for groups push within 1s: %w", err)
		}
		var push proto.Groups
		if proto.Decode(raw, &push) == nil && push.Type == "groups" {
			return m1VerifyGroupListed(push.Groups, w.createdGrp, wantName)
		}
	}
}

func m1VerifyGroupListed(groups []proto.GroupSummary, wantID proto.ID, wantName string) error {
	for _, g := range groups {
		if g.Group == string(wantID) && g.Name == wantName {
			return nil
		}
	}
	return fmt.Errorf("groups push %v missing %s (%s)", groups, wantName, wantID)
}

func (w *m1World) syncSecondAndPostFirst(_, text string) error {
	b1, b2 := w.browsers[0], w.browsers[1]
	buildSync := func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: string(w.createdGrp), Since: 0, StateOnly: false}
	}
	rawSync, err := b2.conn.Request(w.ctx, buildSync)
	if err != nil {
		return err
	}
	var synced proto.Synced
	if err := proto.Decode(rawSync, &synced); err != nil {
		return err
	}
	msgID := exampleID("m1-post-1")
	payload := fmt.Sprintf(`{"t":"post","n":%q,"ts":%d,"g":%q,"kind":"text","text":%q,"thread":null,"main":true,"mentions":[],"re":null,"body":{}}`,
		msgID, w.now.UnixMilli(), w.createdGrp, text)
	env := b1.key.Sign(payload)
	rawCmd, err := b1.conn.Request(w.ctx, func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	})
	if err != nil {
		return err
	}
	var ok proto.Success
	return proto.Decode(rawCmd, &ok)
}

func (w *m1World) checkSecondBrowserMessageRow() error {
	readCtx, cancel := context.WithTimeout(w.ctx, time.Second)
	defer cancel()
	wantID := string(exampleID("m1-post-1"))
	for {
		raw, err := w.browsers[1].nextPush(readCtx)
		if err != nil {
			return fmt.Errorf("waiting for message row within 1s: %w", err)
		}
		if m1HasMessageRow(raw, wantID) {
			return nil
		}
	}
}

func m1HasMessageRow(raw json.RawMessage, wantID string) bool {
	var rows proto.Rows
	if proto.Decode(raw, &rows) != nil || rows.Type != "rows" {
		return false
	}
	return slices.ContainsFunc(rows.Rows, func(r json.RawMessage) bool {
		return m1IsMessageID(r, wantID)
	})
}

func m1IsMessageID(raw json.RawMessage, wantID string) bool {
	var h proto.RowHeader
	if json.Unmarshal(raw, &h) != nil {
		return false
	}
	return h.Type == "msg" && h.ID == wantID
}

func m1SignDeviceAdd(signer identity.Key, user string, ver int, dev, kind, name string, ts int64) proto.Envelope {
	entry := proto.DeviceEntry{
		PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: exampleID(fmt.Sprintf("m1-%s-%d", dev, ver)), Timestamp: ts},
		DeviceChange:  proto.DeviceChange{User: user, Version: ver, Op: "add", Device: dev, Kind: kind, Name: name},
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		panic(err)
	}
	return signer.Sign(string(raw))
}
