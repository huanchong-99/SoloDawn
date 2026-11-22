//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing/fstest"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
)

func init() {
	stepSets["m1"] = m1Steps
}

type m1World struct {
	f          fixtures
	ctx        context.Context
	cleanup    func(func())
	now        time.Time
	rvURL      string
	pn         *nodetest.PeerNode
	accepted   chan *peer.Connection // single reader: acceptInbound
	browsers   []*peerBrowser
	conns      []*peerBrowserConn
	createdGrp proto.ID
}

func m1Steps(sc *godog.ScenarioContext, f fixtures) {
	w := &m1World{
		f:       f,
		cleanup: afterScenario(sc),
		now:     time.Now(),
	}
	sc.Step(`^an M1 owned node with two already-connected browsers of its owner$`, w.setupTwoConnectedBrowsers)
	sc.Step(`^the first M1 browser creates group "([^"]*)"$`, w.createGroupFromFirstBrowser)
	sc.Step(`^the second M1 browser receives a groups push listing "([^"]*)" within 1 second without reconnecting$`, w.checkSecondBrowserGroupsPush)
	sc.Step(`^the second M1 browser syncs "([^"]*)" and the first M1 browser posts "([^"]*)"$`, w.syncSecondAndPostFirst)
	sc.Step(`^the second M1 browser receives that message row within 1 second$`, w.checkSecondBrowserMessageRow)
}

func (w *m1World) setupTwoConnectedBrowsers() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	w.ctx = ctx
	w.cleanup(cancel)
	srv := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
	w.cleanup(srv.Close)
	w.rvURL = "ws" + strings.TrimPrefix(srv.URL, "http") + "/rv"
	w.accepted = make(chan *peer.Connection, 8)
	for i := range 2 {
		b, err := newPeerBrowser(w.ctx, w.cleanup, exampleKey(fmt.Sprintf("m1-browser-%d", i)), w.rvURL, nil)
		if err != nil {
			return err
		}
		w.browsers = append(w.browsers, b)
	}
	tb := scenarioTB{TB: w.f.t, cleanup: w.cleanup}
	w.pn = nodetest.StartPeerNode(tb, nodetest.PeerConfig{
		Ctx:           w.ctx,
		Key:           exampleKey("m1-inproc-node"),
		Name:          "m1-node",
		Version:       "v0.1.0",
		Owner:         w.browsers[0].key,
		ExtraBrowsers: []identity.Key{w.browsers[1].key},
		Rendezvous:    w.rvURL,
		Now:           func() time.Time { return w.now },
		OnInbound:     func(c *peer.Connection) { w.accepted <- c },
	})
	for _, b := range w.browsers {
		if err := w.connectAndHelloBrowser(b); err != nil {
			return err
		}
	}
	return nil
}

func (w *m1World) connectAndHelloBrowser(b *peerBrowser) error {
	bc, err := b.openCtl(w.ctx, w.cleanup, w.pn.Key.ID())
	if err != nil {
		return err
	}
	w.conns = append(w.conns, bc)
	served, err := w.acceptInbound()
	if err != nil {
		return err
	}
	if err := waitConnPath(bc.conn, "direct", time.Second); err != nil {
		return err
	}
	if err := waitConnPath(served, "direct", time.Second); err != nil {
		return err
	}
	if err := checkOwnerWelcome(w.ctx, bc.conn, w.pn.Key.ID()); err != nil {
		return err
	}
	return m1DrainInitialDevlog(w.ctx, bc)
}

func (w *m1World) acceptInbound() (*peer.Connection, error) {
	select {
	case <-w.ctx.Done():
		return nil, w.ctx.Err()
	case c := <-w.accepted:
		return c, nil
	}
}

func m1DrainInitialDevlog(ctx context.Context, bc *peerBrowserConn) error {
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	raw, err := bc.nextPush(readCtx)
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
	b, bc := w.browsers[0], w.conns[0]
	gid := exampleID("m1-group-" + name)
	w.createdGrp = gid
	payload := fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q,"name":%q,"kind":"project","repo":"/tmp/website","base":"main","forge":"none","checks":[],"member_name":"Owner"}`,
		gid, w.now.UnixMilli(), gid, name)
	env := b.key.Sign(payload)
	raw, err := bc.conn.Request(w.ctx, func(id int64) any {
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
		raw, err := w.conns[1].nextPush(readCtx)
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
	b1, bc1, bc2 := w.browsers[0], w.conns[0], w.conns[1]
	rawSync, err := bc2.conn.Request(w.ctx, func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: string(w.createdGrp), Since: 0, StateOnly: false}
	})
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
	rawCmd, err := bc1.conn.Request(w.ctx, func(id int64) any {
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
		raw, err := w.conns[1].nextPush(readCtx)
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
