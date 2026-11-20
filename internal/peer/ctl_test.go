package peer

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestFramingBoundaries(t *testing.T) {
	c := &Connection{ctx: t.Context(), incoming: make(chan json.RawMessage, 1), waiting: make(map[int64]chan json.RawMessage)}
	for _, msg := range []string{`[]`, `{}`, `{"T":"push"}`, `{"t":null}`, `{"re":3}`, `{"re":3,"ok":false}`, `{"id":3,"re":3,"ok":true}`, `{"re":3,"err":{"code":"bad_request"}}`, `{"re":3,"t":"x","err":{"code":"bad_request","msg":"x"}}`, `{"re":3,"ok":false,"err":{"code":"bad_request","msg":"x"}}`, string([]byte{255})} {
		if err := c.deliver([]byte(msg)); err == nil {
			t.Fatalf("accepted %q", msg)
		}
	}
	push := func(n int) string { return `{"t":"` + strings.Repeat("a", n-8) + `"}` }
	check(t, c.deliver([]byte(push(61440))))
	got, err := c.Read(t.Context())
	check(t, err)
	eq(t, string(got), push(61440))
	eq(t, c.deliver([]byte(push(61441))).Error(), "invalid ctl text")
	eq(t, c.Send(json.RawMessage(push(61441))).Error(), "ctl frame exceeds 60 KiB")
	for _, result := range []string{`"ok":true`, `"err":{"code":"bad_request","msg":"invalid"}`} {
		id, ch := c.reserve()
		msg := string(encode(id))
		data := `{"re":` + msg + "," + result + "}"
		check(t, c.deliver([]byte(data)))
		eq(t, string(<-ch), data)
		check(t, c.deliver([]byte(data)))
		eq(t, len(ch), 0)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = c.Read(canceled)
	eq(t, err, context.Canceled)
	c.ctx = canceled
	_, err = c.Read(t.Context())
	eq(t, err, io.EOF)
}
func TestCtlSizeAndReplyRouting(t *testing.T) {
	f := newPair(t)
	n := f.open(t, 0)
	payload := map[string]string{"t": "push", "data": ""}
	payload["data"] = strings.Repeat("a", 61440-len(encode(payload)))
	check(t, n.client.Send(payload))
	data, err := n.server.Read(t.Context())
	check(t, err)
	eq(t, len(data), 61440)
	eq(t, string(data), string(encode(payload)))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = n.client.Request(ctx, func(id int64) any { return proto.Request{Type: "one", ID: id} })
	eq(t, err, context.Canceled)
	data, err = n.server.Read(t.Context())
	check(t, err)
	eq(t, string(data), `{"t":"one","id":1}`)
}
func TestBackoffSequenceAndWake(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		delay := time.Second
		for _, seconds := range []int{1, 2, 4, 8, 16, 32, 60, 60} {
			start := time.Now()
			delay = backoff(t.Context(), nil, delay)
			eq(t, time.Since(start), time.Duration(seconds)*time.Second)
		}
		wake := make(chan struct{}, 1)
		wake <- struct{}{}
		start := time.Now()
		eq(t, backoff(t.Context(), wake, delay), time.Second)
		eq(t, time.Since(start), time.Duration(0))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		eq(t, backoff(ctx, nil, delay), time.Duration(0))
	})
}
func TestPresenceRestartsUnansweredNegotiationImmediately(t *testing.T) {
	f := newPair(t)
	wake := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() { f.a.Connect(ctx, f.b.key.ID(), wake, func(*Connection) {}); close(done) }()
	a := recv(t, f.aMail["offer"])
	wake <- struct{}{}
	b := recv(t, f.aMail["offer"])
	var x, y proto.Offer
	check(t, proto.Decode([]byte(a.Payload), &x))
	check(t, proto.Decode([]byte(b.Payload), &y))
	if x.Conn == y.Conn {
		t.Fatal("new attempt reused the old conn")
	}
	cancel()
	recv(t, done)
}
