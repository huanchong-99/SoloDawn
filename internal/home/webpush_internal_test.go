package home

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func wpushIntKey(seed byte) identity.Key {
	return identity.Key(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize)))
}

func wpushIntID(seed byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{seed}, 16))
}

func wpushIntP256(t *testing.T, seed byte) *ecdh.PrivateKey {
	t.Helper()
	priv, err := ecdh.P256().NewPrivateKey(bytes.Repeat([]byte{seed}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return priv
}

func wpushIntSub(t *testing.T, user string, dev identity.Key, seed byte, endpoint string) wpushSub {
	t.Helper()
	ua := wpushIntP256(t, seed)
	vapid := wpushIntP256(t, seed+10)
	return wpushSub{
		device: dev.ID(),
		user:   user,
		sub: proto.PushSubscription{
			Endpoint: endpoint,
			P256DH:   base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes()),
			Auth:     base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{seed + 20}, 16)),
		},
		vapid: proto.VAPIDKeys{
			Public:  base64.RawURLEncoding.EncodeToString(vapid.PublicKey().Bytes()),
			Private: base64.RawURLEncoding.EncodeToString(vapid.Bytes()),
		},
		events: []string{"mention"},
	}
}

func TestWebPushBrowserLogClassification(t *testing.T) {
	b1 := wpushIntKey(101)
	n1 := wpushIntKey(102)
	if wpushBrowser(nil, b1.ID()) {
		t.Fatal("nil log must return false")
	}
	log := &identity.Log{User: b1.ID()}
	for i, it := range []struct {
		dev  identity.Key
		kind string
	}{{b1, "browser"}, {n1, "node"}, {b1, "node"}} {
		p := fmt.Sprintf(`{"t":"device","user":%q,"ver":%d,"op":"add","dev":%q,"kind":%q,"name":"d"}`, b1.ID(), i+1, it.dev.ID(), it.kind)
		if _, err := log.Apply(b1.Sign(p)); err != nil {
			t.Fatal(err)
		}
	}
	if wpushBrowser(log, b1.ID()) || wpushBrowser(log, n1.ID()) {
		t.Fatal("retyped browser and node must return false")
	}
}

func TestWebPushEvictionInMemoryDeadlineAndLoopCloseWait(t *testing.T) {
	homeID := wpushIntKey(110).ID()
	b1 := wpushIntKey(111)
	b2 := wpushIntKey(112)
	ownerMem := wpushIntID(113)
	grpID := wpushIntID(114)

	s := NewState(grpID, homeID, b1.ID())
	log := &identity.Log{User: b1.ID()}
	for i, d := range []identity.Key{b1, b2} {
		p := fmt.Sprintf(`{"t":"device","user":%q,"ver":%d,"op":"add","dev":%q,"kind":"browser","name":"b"}`, b1.ID(), i+1, d.ID())
		log.Apply(b1.Sign(p))
	}
	s.Logs[b1.ID()] = log
	s.Group = proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: grpID}, Name: "G", Home: homeID}
	s.People[ownerMem] = proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: ownerMem}, User: b1.ID(), Name: "Owner"}, Role: "owner"}

	var mu sync.Mutex
	var hits []string
	var fresh wpushSub
	hangEntered := make(chan struct{}) // single reader: test goroutine below
	hangRelease := make(chan struct{}) // closed once after client timeout completes
	status := map[string]int{"/500": http.StatusInternalServerError, "/404": http.StatusNotFound, "/410": http.StatusGone}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/hang" {
			close(hangEntered)
			<-hangRelease
			return
		}
		mu.Lock()
		hits = append(hits, req.URL.Path)
		replace := req.URL.Path == "/410"
		code := status[req.URL.Path]
		mu.Unlock()
		if replace {
			wpushApplyWrite(homeID, wpushPut{sub: fresh})
		}
		if code == 0 {
			code = http.StatusCreated
		}
		rw.WriteHeader(code)
	}))
	defer srv.Close()

	fresh = wpushIntSub(t, b1.ID(), b2, 15, srv.URL+"/fresh")
	mention := func(n byte) Result {
		return Result{Rows: []Row{RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: wpushIntID(n), Seq: int64(n)}, Pos: int64(n),
			Group: grpID, By: ownerMem, Kind: "text", Mentions: []string{ownerMem}, Body: json.RawMessage(`{}`),
		})}}
	}

	// Context timeout (returning while /hang is still held) and 500 both preserve subscriptions.
	prevWindow := wpushWindow
	wpushWindow = 25 * time.Millisecond
	t.Cleanup(func() { wpushWindow = prevWindow })
	wpushApplyWrite(homeID, wpushPut{sub: wpushIntSub(t, b1.ID(), b1, 11, srv.URL+"/hang")})
	wpushApplyWrite(homeID, wpushPut{sub: wpushIntSub(t, b1.ID(), b2, 12, srv.URL+"/500")})
	wpushAfter(s, mention(1))
	<-hangEntered
	wpushFor(homeID).wg.Wait()
	close(hangRelease)
	wpushWindow = prevWindow
	if len(wpushFor(homeID).subs) != 2 {
		t.Fatalf("expected 2 kept subs after timeout and 500, got %d", len(wpushFor(homeID).subs))
	}

	// 404 evicts b1 in memory; 410 on b2 while replaced by fresh preserves fresh.
	wpushApplyWrite(homeID, wpushPut{sub: wpushIntSub(t, b1.ID(), b1, 13, srv.URL+"/404")})
	wpushApplyWrite(homeID, wpushPut{sub: wpushIntSub(t, b1.ID(), b2, 14, srv.URL+"/410")})
	wpushAfter(s, mention(2))
	wpushFor(homeID).wg.Wait()

	hits = nil
	wpushAfter(s, mention(3))
	wpushFor(homeID).wg.Wait()
	if len(hits) != 1 || hits[0] != "/fresh" {
		t.Fatalf("expected only /fresh after 404 and Concurrent replace, got %v", hits)
	}

	wpushVerifyCloseWaits(t, b1, ownerMem, grpID)
}

func wpushVerifyCloseWaits(t *testing.T, b1 identity.Key, ownerMem, grpID string) {
	t.Helper()
	entered := make(chan struct{})  // single reader: test goroutine below
	release := make(chan struct{})  // closed once by test goroutine
	closedCh := make(chan struct{}) // single reader: select check below
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		rw.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	homeID := wpushIntKey(120).ID()
	loop, err := StartLoop(LoopConfig{
		Store: db, Home: homeID, Owner: b1.ID(),
		Logs:   func(string) *identity.Log { return nil },
		Tables: NodeTables(), Now: func() time.Time { return time.UnixMilli(1763600000000) },
	})
	if err != nil {
		t.Fatal(err)
	}
	s := NewState(grpID, homeID, b1.ID())
	log := &identity.Log{User: b1.ID()}
	log.Apply(b1.Sign(fmt.Sprintf(`{"t":"device","user":%q,"ver":1,"op":"add","dev":%q,"kind":"browser","name":"b"}`, b1.ID(), b1.ID())))
	s.Logs[b1.ID()] = log
	s.Group = proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: grpID}, Name: "G", Home: homeID}
	s.People[ownerMem] = proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: ownerMem}, User: b1.ID(), Name: "Owner"}, Role: "owner"}

	wpushApplyWrite(homeID, wpushPut{sub: wpushIntSub(t, b1.ID(), b1, 16, srv.URL+"/slow")})
	wpushAfter(s, Result{Rows: []Row{RowOf(proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: wpushIntID(10), Seq: 10}, Pos: 10,
		Group: grpID, By: ownerMem, Kind: "text", Mentions: []string{ownerMem}, Body: json.RawMessage(`{}`),
	})}})
	<-entered

	go func() {
		loop.Close()
		close(closedCh)
	}()
	select {
	case <-closedCh:
		t.Fatal("Close returned while a Web Push request was still in flight")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-closedCh
	if wpushFor(homeID).loop != nil {
		t.Fatal("Close must detach loop from wpushHome")
	}
}
