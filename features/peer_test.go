//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Node-level host information is a fixture; the requests, answers, pushes,
// signatures, signalling sockets and data channels here are real.
const peerInfo = `"device":"%s","name":"test node","os":"test","version":"1","open":0,"max_open":4,"offline":false,"update":null,"homes":[],"relay":{"used":0,"limit":0},"agents":[],"settings":{"retention_days":90,"idle_minutes":15,"default_level":2,"all_level3":false,"approvers":[]}`

type peersWorld struct {
	ctx     context.Context
	cancel  context.CancelFunc
	nodes   [2]*peer.Peer
	ids     [2]string
	conns   [2]*peer.Connection
	cleanup []func()
}

func init() {
	stepSets["peer"] = func(sc *godog.ScenarioContext, _ fixtures) { peerSteps(sc) }
}

func peerSteps(sc *godog.ScenarioContext) {
	w := &peersWorld{}
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		for _, undo := range slices.Backward(w.cleanup) {
			undo()
		}
		return ctx, err
	})
	sc.Step("^two authorized nodes signed in on two local signalling paths$", w.setup)
	sc.Step("^each node offers the other a connection$", w.offer)
	sc.Step("^both directions are direct and answer host.info$", w.info)
	sc.Step("^an ordered ctl push follows the reply in each direction$", w.push)
}
func (w *peersWorld) setup() error {
	w.ctx, w.cancel = context.WithTimeout(context.Background(), 15*time.Second)
	w.cleanup = append(w.cleanup, w.cancel)
	names := []string{"node", "tablet"}
	var paths [2][]*peer.Path
	for range 2 {
		srv := httptest.NewServer(rendezvous.Handler(rvWeb))
		w.cleanup = append(w.cleanup, srv.Close)
		for i, name := range names {
			key := exampleKey(name)
			w.ids[i] = key.ID()
			p, err := peer.Dial(w.ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/rv", key)
			if err != nil {
				return err
			}
			w.cleanup = append(w.cleanup, func() { p.Close() })
			paths[i] = append(paths[i], p)
		}
	}
	for i, name := range names {
		socks, err := peer.Listen(0, 0)
		if err != nil {
			return err
		}
		db, err := store.Open(":memory:")
		if err != nil {
			return err
		}
		w.cleanup = append(w.cleanup, func() { socks.Close(); db.Close() })
		w.nodes[i] = peer.New(exampleKey(name), socks, db, func(id string) bool { return id == w.ids[1-i] }, func(string) []peer.Route { return []peer.Route{paths[i][0].Route(), paths[i][1].Route()} }, func(c *peer.Connection) { w.serve(c, w.ids[i]) })
		for _, p := range paths[i] {
			go p.Serve(w.ctx, w.nodes[i], func(string, []byte) error { return nil })
		}
	}
	return nil
}
func (w *peersWorld) serve(c *peer.Connection, id string) {
	defer c.Close()
	data, err := c.Read(w.ctx)
	if err != nil {
		return
	}
	var q proto.Request
	if proto.Decode(data, &q) != nil || q.Type != "host.info" {
		return
	}
	fields := fmt.Sprintf(peerInfo, id)
	if c.Send(json.RawMessage(fmt.Sprintf(`{"t":"host.info","re":%d,"ok":true,%s}`, q.ID, fields))) != nil {
		return
	}
	if c.Send(json.RawMessage(`{"t":"host.info",`+fields+"}")) != nil {
		return
	}
	select {
	case <-w.ctx.Done():
	case <-c.Done():
	}
}
func (w *peersWorld) offer() error {
	for i, node := range w.nodes {
		c, err := node.Offer(w.ctx, w.ids[1-i])
		if err != nil {
			return err
		}
		w.conns[i] = c
		w.cleanup = append(w.cleanup, func() { c.Close() })
	}
	return nil
}
func (w *peersWorld) info() error {
	for i, c := range w.conns {
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case <-c.Ready():
		}
		if path, err := c.Path(); err != nil || path != "direct" {
			return fmt.Errorf("path %q: %v", path, err)
		}
		data, err := c.Request(w.ctx, func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
		if err != nil {
			return err
		}
		want := fmt.Sprintf(`{"t":"host.info","re":1,"ok":true,`+peerInfo+"}", w.ids[1-i])
		if string(data) != want {
			return fmt.Errorf("answer: %s, want %s", data, want)
		}
	}
	return nil
}
func (w *peersWorld) push() error {
	for i, c := range w.conns {
		data, err := c.Read(w.ctx)
		if err != nil {
			return err
		}
		want := fmt.Sprintf(`{"t":"host.info",`+peerInfo+"}", w.ids[1-i])
		if string(data) != want {
			return fmt.Errorf("push: %s, want %s", data, want)
		}
	}
	return nil
}
