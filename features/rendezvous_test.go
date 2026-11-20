//go:build acceptance

package features

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing/fstest"
	"time"

	"github.com/cucumber/godog"
	"github.com/gorilla/websocket"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
)

// Change one byte of the signed description on the server's outgoing socket.
// The uncompressed server frame stays the same length and valid JSON.
type testRVListener struct {
	net.Listener
	change atomic.Bool
}

func (l *testRVListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	return testRVConn{c, &l.change}, err
}

type testRVConn struct {
	net.Conn
	change *atomic.Bool
}

func (c testRVConn) Write(b []byte) (int, error) {
	if c.change.Load() {
		b = bytes.ReplaceAll(b, []byte("98:51:B3:F7"), []byte("99:51:B3:F7"))
	}
	return c.Conn.Write(b)
}

func rvLogin(t godog.TestingT, url string, key identity.Key) *websocket.Conn {
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	var challenge proto.Challenge
	if err := c.ReadJSON(&challenge); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteJSON(proto.Auth{Type: "auth", Key: key.ID(), Signature: key.SignChallenge(challenge.Nonce)}); err != nil {
		t.Fatal(err)
	}
	var ready proto.ICEConfig
	if err := c.ReadJSON(&ready); err != nil || ready.Type != "ready" {
		t.Fatalf("ready = %+v, %v", ready, err)
	}
	return c
}

func rendezvousSteps(sc *godog.ScenarioContext) {
	var srv *httptest.Server
	var listener *testRVListener
	var sender, receiver *websocket.Conn
	browser, node := exampleKey("browser"), exampleKey("node")
	var got proto.Envelope
	var cleanup []func()
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		for _, close := range slices.Backward(cleanup) {
			close()
		}
		return ctx, err
	})
	sc.Step(`^two devices authenticated to a local rendezvous$`, func(ctx context.Context) {
		srv = httptest.NewUnstartedServer(rendezvous.Handler(fstest.MapFS{}))
		listener = &testRVListener{Listener: srv.Listener}
		srv.Listener = listener
		srv.Start()
		cleanup = append(cleanup, srv.Close)
		url := strings.Replace(srv.URL, "http:", "ws:", 1) + "/rv"
		sender = rvLogin(godog.T(ctx), url, browser)
		cleanup = append(cleanup, func() { sender.Close() })
		receiver = rvLogin(godog.T(ctx), url, node)
		cleanup = append(cleanup, func() { receiver.Close() })
	})
	sc.Step(`^the first offers a connection and the rendezvous (changes|preserves) the SDP fingerprint in transit$`, func(action string) error {
		listener.change.Store(action == "changes")
		if err := rvOffer(sender, browser, node.ID()); err != nil {
			return err
		}
		return rvReceive(receiver, browser.ID(), &got)
	})
	sc.Step(`^the second device's offer check returns "([^"]*)"$`, func(want string) error {
		result := "accepted"
		known := func(id string) bool { return id == browser.ID() }
		if _, err := identity.OpenSignal(got, node.ID(), known, time.Now()); err != nil {
			result = err.Error()
		}
		if result != want {
			return fmt.Errorf("offer check = %s, want %s", result, want)
		}
		return nil
	})
}

func rvOffer(c *websocket.Conn, signer identity.Key, target string) error {
	var offer proto.Offer
	if err := readWireExample("offer", &offer); err != nil {
		return err
	}
	offer.Timestamp = time.Now().UnixMilli()
	payload, err := json.Marshal(offer)
	if err != nil {
		return err
	}
	env, err := json.Marshal(signer.Sign(string(payload)))
	if err != nil {
		return err
	}
	return c.WriteJSON(proto.Route{Type: "to", Key: target, Env: env})
}

func rvReceive(c *websocket.Conn, sender string, got *proto.Envelope) error {
	var from proto.Route
	if err := c.ReadJSON(&from); err != nil {
		return err
	}
	if from.Type != "from" || from.Key != sender {
		return fmt.Errorf("unexpected route: %+v", from)
	}
	return proto.Decode(from.Env, got)
}
