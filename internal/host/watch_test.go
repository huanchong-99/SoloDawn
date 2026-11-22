package host

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"testing/synctest"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func TestWatchLifecycle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newPTYTestHost(t)
		p := watchState(h)
		slot := aiSlot{ptyTestGroupID, ptyTestAIID}
		p.grants[slot] = map[string]string{"z": "member-z", "a": "member-a"}
		if got := h.Watchers(slot.group, slot.member); !reflect.DeepEqual(got, []string{"a", "z"}) {
			t.Fatal(got)
		}
		h.Rows.Put(slot.group, "device", "a:1", json.RawMessage(marshalWire(proto.DeviceRow{RowHeader: proto.RowHeader{Type: "device", ID: "a:1"}, DeviceChange: proto.DeviceChange{User: "a", Version: 1, Op: "add", Device: "owner-browser"}})))
		data := json.RawMessage(marshalWire(proto.WatchStart{Request: proto.Request{Type: "watch.start", ID: 1}, Group: proto.ID(slot.group), Member: proto.ID(slot.member)}))
		o := newTkOwner("relay")
		_, err := h.WatchStart(o, "owner-browser", data)
		tkCode(t, err, "relay")
		o.setPath("direct")
		if res, err := h.WatchStart(o, "owner-browser", data); err != nil || res != nil {
			t.Fatalf("%v %v", res, err)
		}
		if _, err := h.WatchStart(o, "owner-browser", data); err != nil {
			t.Fatal(err)
		}
		term := newPTYTestTerminal("watch:" + slot.group + ":" + slot.member)
		ptyState(h).noteSize(slot, 80, 24)
		go h.ServeWatch(term)
		synctest.Wait()
		duplicate := newPTYTestTerminal(term.label)
		h.ServeWatch(duplicate)
		if !duplicate.isClosed() {
			t.Fatal("duplicate open")
		}
		var owner bytes.Buffer
		n, err := (watchFanout{&owner, p, slot}).Write([]byte("hello"))
		if n != 5 || err != nil || owner.String() != "hello" {
			t.Fatalf("%d %v %q", n, err, owner.String())
		}
		synctest.Wait()
		if got := term.output(); got != "hello" {
			t.Fatal(got)
		}
		o.setPath("relay")
		synctest.Wait()
		if !term.isClosed() {
			t.Fatal("relay left watch open")
		}
		if got := h.Watchers(slot.group, slot.member); !reflect.DeepEqual(got, []string{"a", "z"}) {
			t.Fatal(got)
		}
		o.setPath("direct")
		if _, err := h.WatchStart(o, "owner-browser", data); err != nil {
			t.Fatal(err)
		}
		term = newPTYTestTerminal("watch:" + slot.group + ":" + slot.member)
		go h.ServeWatch(term)
		synctest.Wait()
		if _, err := term.inW.Write([]byte("i")); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if !term.isClosed() {
			t.Fatal("input accepted")
		}
		close(o.done)
		synctest.Wait()
		watchGone(h, slot.group, "member-z")
		if got := h.Watchers(slot.group, slot.member); !reflect.DeepEqual(got, []string{"a"}) {
			t.Fatal(got)
		}
		p.stop(slot)
		if got := h.Watchers(slot.group, slot.member); !reflect.DeepEqual(got, []string{}) {
			t.Fatal(got)
		}
	})
}

func TestWatchItems(t *testing.T) {
	h := newPTYTestHost(t)
	slot := aiSlot{ptyTestGroupID, ptyTestAIID}
	key := SessionKey{Group: slot.group, Member: slot.member}
	if err := h.Sessions.Create(Session{SessionKey: key, Kind: GroupSession, Agent: "claude"}); err != nil {
		t.Fatal(err)
	}
	v := verifiedItem{group: slot.group, home: "home", item: proto.Item{Item: 1, AI: proto.ID(slot.member), Why: "watch"}}
	dispatchArrivedItem(h, nil, v)
	if got := h.Watchers(slot.group, slot.member); !reflect.DeepEqual(got, []string{}) {
		t.Fatal(got)
	}
	if err := h.Sessions.Move(key, Dormant, Takeover); err != nil {
		t.Fatal(err)
	}
	h.Rows.Put(slot.group, "member", "by", json.RawMessage(marshalWire(proto.MemberRow{RowHeader: proto.RowHeader{ID: "by"}, User: "watcher"})))
	v.item.Item = 2
	v.wire.Msg = json.RawMessage(marshalWire(proto.MsgRow{By: "by", Mentions: []string{}}))
	dispatchArrivedItem(h, nil, v)
	dispatchArrivedItem(h, nil, v)
	if got := h.Watchers(slot.group, slot.member); !reflect.DeepEqual(got, []string{"watcher"}) {
		t.Fatal(got)
	}
	var count int
	if err := h.Store.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT count(*) FROM audit WHERE what = 'delivered' AND detail = 'watch'").Scan(&count)
	}); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal(count)
	}
	h.Rows.Put(slot.group, "member", "by", json.RawMessage(marshalWire(proto.GoneRow{RowHeader: proto.RowHeader{ID: "by"}, Gone: true})))
	cleanupGoneMembers(h, slot.group)
	if got := h.Watchers(slot.group, slot.member); !reflect.DeepEqual(got, []string{}) {
		t.Fatal(got)
	}
	if err := h.Store.Write(func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE audit"); return err }); err != nil {
		t.Fatal(err)
	}
	watchItem(h, v)
}

func TestWatchInvalidStoredRows(t *testing.T) {
	for _, kind := range []string{"card", "device"} {
		t.Run(kind, func(t *testing.T) {
			h := newPTYTestHost(t)
			defer func() {
				if recover() == nil {
					t.Fatal("invalid row did not panic")
				}
			}()
			if kind == "device" {
				h.Rows.Put("g", "device", "d", json.RawMessage(`{`))
				watchDeviceUser(h, "g", "d")
				return
			}
			key := SessionKey{Group: "g", Member: "m"}
			if err := h.Sessions.Create(Session{SessionKey: key, Kind: GroupSession, Agent: "claude"}); err != nil {
				t.Fatal(err)
			}
			if err := h.Sessions.Move(key, Dormant, Takeover); err != nil {
				t.Fatal(err)
			}
			watchItem(h, verifiedItem{group: "g", item: proto.Item{AI: "m"}, wire: proto.InboxItem{Msg: json.RawMessage(`{`)}})
		})
	}
}

func TestWatchRefusalsAndRevocation(t *testing.T) {
	h := newPTYTestHost(t)
	o := newTkOwner("direct")
	_, err := h.WatchStart(o, "dev", json.RawMessage(`{`))
	tkCode(t, err, "bad_request")
	data := json.RawMessage(marshalWire(proto.WatchStart{Request: proto.Request{Type: "watch.start", ID: 1}, Group: ptyTestGroupID, Member: ptyTestAIID}))
	_, err = h.WatchStart(o, "dev", data)
	tkCode(t, err, "forbidden")
	for _, label := range []string{"wrong", "watch:g:", "watch:g:m", "g:m"} {
		term := newPTYTestTerminal(label)
		h.ServeWatch(term)
		if !term.isClosed() {
			t.Fatal(label)
		}
	}
	_, err = watchEnd(context.Background(), h, o, "dev", json.RawMessage(`{`))
	tkCode(t, err, "bad_request")
	end := json.RawMessage(marshalWire(proto.WatchEnd{HostCommand: proto.HostCommand{Type: "watch.end", Nonce: proto.NewNonce()}, Group: ptyTestGroupID, Member: ptyTestAIID, User: "a"}))
	_, err = watchEnd(context.Background(), h, o, "dev", end)
	tkCode(t, err, "not_found")
	p := watchState(h)
	slot := aiSlot{ptyTestGroupID, ptyTestAIID}
	p.grants[slot] = map[string]string{"a": "by"}
	key := watchKey{slot, "dev"}
	p.ok[key] = watchOK{o, "a"}
	term := newPTYTestTerminal("unused")
	if _, ok := p.attach(key, term); !ok {
		t.Fatal("attach")
	}
	if res, err := watchEnd(context.Background(), h, o, "dev", end); res != nil || err != nil {
		t.Fatalf("%v %v", res, err)
	}
	if !term.isClosed() {
		t.Fatal("revoke left channel open")
	}
	if got := h.Watchers(slot.group, slot.member); !reflect.DeepEqual(got, []string{}) {
		t.Fatal(got)
	}
}

func TestWatchQueueLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newPTYTestHost(t)
		p := watchState(h)
		slot := aiSlot{"group", "member"}
		key := watchKey{slot, "dev"}
		p.ok[key] = watchOK{newTkOwner("direct"), "a"}
		term := newPTYTestTerminal("unused")
		term.setBlocked(true)
		w, _ := p.attach(key, term)
		go p.writeLoop(w)
		p.fanout(aiSlot{"other", "member"}, []byte("ignored"))
		p.fanout(slot, bytes.Repeat([]byte("a"), 1<<20))
		synctest.Wait()
		if term.isClosed() {
			t.Fatal("closed at limit")
		}
		p.fanout(slot, []byte("b"))
		synctest.Wait()
		if !term.isClosed() {
			t.Fatal("overflow left channel open")
		}
		p.stop(slot)
	})
}
