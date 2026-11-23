package node

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestHostInfoRefreshPushesOnlyToWelcomedOwnerDevices(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, ownerKey := sesKey("model-node"), sesKey("model-owner")
	logs := seedOwnedLogs(t, db, nodeKey, ownerKey)
	loop, h := newTestHomeAndHost(t, db, nodeKey, logs, time.UnixMilli(1763600000000), nil)
	s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h})
	owner, guest, unwelcomed := newFakeCtl(), newFakeCtl(), newFakeCtl()
	s.conns[&sesConn{device: ownerKey.ID(), welcomed: true, raw: owner}] = true
	s.conns[&sesConn{device: "guest", welcomed: true, raw: guest}] = true
	s.conns[&sesConn{device: ownerKey.ID(), raw: unwelcomed}] = true
	var seen []bool
	var listErr error
	sf := NewSurface(SurfaceConfig{ID: nodeKey.ID(), Store: db, Host: h, Sessions: s,
		Agents: func(_ context.Context, refresh bool) ([]proto.HostAgent, error) {
			seen = append(seen, refresh)
			return []proto.HostAgent{}, listErr
		},
	})
	read := func(text string) (any, error) {
		return sf.reqHostInfo(t.Context(), nil, owner, ownerKey.ID(), 1, json.RawMessage(text))
	}
	state, err := read(`{"t":"host.info","id":1}`)
	mustNoErr(t, err)
	mustEqual(t, len(owner.out), 0)
	fresh, err := read(`{"t":"host.info","id":2,"refresh":true}`)
	mustNoErr(t, err)
	mustEqual(t, fresh, state)
	var push proto.HostInfoPush
	mustNoErr(t, proto.Decode(owner.next(t), &push))
	mustEqual(t, push, proto.HostInfoPush{Type: "host.info", HostState: fresh.(proto.HostState)})
	mustEqual(t, []int{len(guest.out), len(unwelcomed.out)}, []int{0, 0})
	_, err = read(`{"t":"host.info","id":3,"refresh":"true"}`)
	mustEqual(t, err.(*host.Error).Code, "bad_request")
	listErr = io.ErrClosedPipe
	_, err = read(`{"t":"host.info","id":4,"refresh":true}`)
	mustEqual(t, err, io.ErrClosedPipe)
	mustEqual(t, seen, []bool{false, true, true})
	mustEqual(t, len(owner.out), 0)
}

func TestStartupStopsWhenModelDiscoveryIsCanceled(t *testing.T) {
	h, _, err := host.New(sesOpenStore(t), sesKey("model-startup"), time.Now)
	mustNoErr(t, err)
	t.Cleanup(h.Settle)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	n := nodeRuntime{ctx: ctx, host: h}
	mustEqual(t, n.stepStartup(), context.Canceled)
}
