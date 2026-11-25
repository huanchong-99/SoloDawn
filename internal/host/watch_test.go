package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"testing/synctest"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type watchOwner struct {
	*tkOwner
	sizes []proto.PTYResize
}

func (o *watchOwner) Send(v any) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sizes = append(o.sizes, v.(proto.PTYResize))
	return nil
}

func (o *watchOwner) wantSizes(t *testing.T, want ...proto.PTYResize) {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	if !reflect.DeepEqual(o.sizes, want) {
		t.Fatalf("sizes = %+v, want %+v", o.sizes, want)
	}
}

func TestWatchLifecycle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := captureDiag(t)
		h := newPTYTestHost(t)
		p := watchState(h)
		slot := aiSlot{ptyTestGroupID, ptyTestAIID}
		p.grants[slot] = map[string]string{"z": "member-z", "a": "member-a"}
		if got := h.Watchers(slot.group, slot.member); !reflect.DeepEqual(got, []string{"a", "z"}) {
			t.Fatal(got)
		}
		h.Rows.Put(slot.group, "device", "a:1", json.RawMessage(marshalWire(proto.DeviceRow{RowHeader: proto.RowHeader{Type: "device", ID: "a:1"}, DeviceChange: proto.DeviceChange{User: "a", Version: 1, Op: "add", Device: "owner-browser"}})))
		data := json.RawMessage(marshalWire(proto.WatchStart{Request: proto.Request{Type: "watch.start", ID: 1}, Group: proto.ID(slot.group), Member: proto.ID(slot.member)}))
		o := &watchOwner{tkOwner: newTkOwner("relay")}
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
		part := ptyState(h)
		entry := &PTY{part: part, slot: slot, handle: ptyNopHandle{}}
		part.activate(slot, entry)
		sendWatchSize(h, slot, o)
		o.wantSizes(t)
		size := proto.PTYResize{Type: "pty.resize", Group: proto.ID(slot.group), AI: proto.ID(slot.member), Cols: 80, Rows: 24}
		assertEqual(t, entry.resize(size), nil)
		go h.ServeWatch(term)
		synctest.Wait()
		o.wantSizes(t, size)
		h.Rows.Put(slot.group, "device", "a:2", json.RawMessage(marshalWire(proto.DeviceRow{RowHeader: proto.RowHeader{Type: "device", ID: "a:2"}, DeviceChange: proto.DeviceChange{User: "a", Version: 2, Op: "add", Device: "second-browser"}})))
		second := &watchOwner{tkOwner: newTkOwner("direct")}
		if _, err := h.WatchStart(second, "second-browser", data); err != nil {
			t.Fatal(err)
		}
		secondTerm := newPTYTestTerminal(term.label)
		secondTerm.device = "second-browser"
		go h.ServeWatch(secondTerm)
		synctest.Wait()
		o.wantSizes(t, size)
		second.wantSizes(t, size)
		resized := size
		resized.Cols, resized.Rows = 100, 40
		assertEqual(t, entry.resize(resized), nil)
		o.wantSizes(t, size, resized)
		second.wantSizes(t, size, resized)
		secondTerm.Close()
		close(second.done)
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
		part.release(slot)
		sendWatchSize(h, slot, o)
		o.wantSizes(t, size, resized, resized)
		node := "node=" + h.Key.ID()[:8]
		d.ran(t, 3, node, "watch_conn")
		d.ran(t, 3, node, "watch_write g="+slot.group)
	})
}

// recordWatchers keeps the watchers of a slot at each notice; reading them shows that no lock is held.
func recordWatchers(h *Host, slot aiSlot) *[][]string {
	seen := &[][]string{}
	h.WatchersChanged = func() { *seen = append(*seen, h.Watchers(slot.group, slot.member)) }
	return seen
}

func wantWatchers(t *testing.T, seen *[][]string, want ...[]string) {
	t.Helper()
	if !reflect.DeepEqual(*seen, append([][]string{}, want...)) {
		t.Fatalf("watchers at each notice: %v, want %v", *seen, want)
	}
}

func TestWatchItems(t *testing.T) {
	h := newPTYTestHost(t)
	slot := aiSlot{ptyTestGroupID, ptyTestAIID}
	seen := recordWatchers(h, slot)
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
	v.wire.Msg = json.RawMessage(`{`)
	watchItem(h, v)
	if got := h.Watchers(slot.group, slot.member); !reflect.DeepEqual(got, []string{}) {
		t.Fatal(got)
	}
	wantWatchers(t, seen)
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
	cleanupGoneMembers(h, slot.group)
	wantWatchers(t, seen, []string{"watcher"}, []string{"watcher"}, []string{})
	if got := h.Watchers(slot.group, slot.member); !reflect.DeepEqual(got, []string{}) {
		t.Fatal(got)
	}
	if err := h.Store.Write(func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE audit"); return err }); err != nil {
		t.Fatal(err)
	}
	watchItem(h, v)
	if got := h.Watchers(slot.group, slot.member); !reflect.DeepEqual(got, []string{}) {
		t.Fatal(got)
	}
	if h.Watching("stranger") {
		t.Fatal("gone member granted an unknown device")
	}
}

func TestWatchRefusalsAndRevocation(t *testing.T) {
	h := newPTYTestHost(t)
	h.Rows.Put("g", "device", "bad", json.RawMessage(`{`))
	if got := watchDeviceUser(h, "g", "dev"); got != "" {
		t.Fatal(got)
	}
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
	if _, from := p.attach(key, term); from != o {
		t.Fatal("attach")
	}
	seen := recordWatchers(h, slot)
	if res, err := watchEnd(context.Background(), h, o, "dev", end); res != nil || err != nil {
		t.Fatalf("%v %v", res, err)
	}
	wantWatchers(t, seen, []string{})
	if !term.isClosed() {
		t.Fatal("revoke left channel open")
	}
	if got := h.Watchers(slot.group, slot.member); !reflect.DeepEqual(got, []string{}) {
		t.Fatal(got)
	}
}

func TestWatchFanoutError(t *testing.T) {
	h := newPTYTestHost(t)
	want := errors.New("owner write failed")
	n, err := (watchFanout{failWriter{want}, watchState(h), aiSlot{"g", "m"}}).Write([]byte("hello"))
	if n != 0 || err != want {
		t.Fatalf("Write = %d, %v", n, err)
	}
}

func TestWatchHandBack(t *testing.T) {
	h := newPTYTestHost(t)
	registerPTYTestDriver(t, "watch-stop", func(Launch) ([]string, string, error) {
		return ptyHelperArgv(t, "pty-echo"), "watch-stop", nil
	})
	pty, _, err := h.StartPTY("g", "m", "watch-stop", Launch{Folder: t.TempDir()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := watchState(h)
	slot := aiSlot{"g", "m"}
	p.grants[slot] = map[string]string{"a": "by"}
	key := watchKey{slot, "dev"}
	p.ok[key] = watchOK{newTkOwner("direct"), "a"}
	term := newPTYTestTerminal("unused")
	p.attach(key, term)
	var ended []string
	h.WatchEnded = func(group, user string) { ended = append(ended, group+":"+user) }
	seen := recordWatchers(h, slot)
	if err := pty.Close(); err != nil {
		t.Fatal(err)
	}
	wantWatchers(t, seen, []string{})
	p.stop(slot)
	wantWatchers(t, seen, []string{})
	if !term.isClosed() || len(p.ok) != 0 || !reflect.DeepEqual(h.Watchers("g", "m"), []string{}) || !reflect.DeepEqual(ended, []string{"g:a"}) {
		t.Fatalf("closed=%v, ok=%v, watchers=%v, ended=%v", term.isClosed(), p.ok, h.Watchers("g", "m"), ended)
	}
}

func TestWatchAdmission(t *testing.T) {
	h := newPTYTestHost(t)
	p := watchState(h)
	p.grants[aiSlot{"g", "m"}] = map[string]string{"a": "by"}
	p.grants[aiSlot{"other", "m"}] = map[string]string{"a": "by"}
	for _, group := range []string{"g", "other"} {
		h.Rows.Put(group, "device", "a:1", json.RawMessage(marshalWire(proto.DeviceRow{RowHeader: proto.RowHeader{Type: "device", ID: "a:1"}, DeviceChange: proto.DeviceChange{User: "a", Version: 1, Op: "add", Device: "dev"}})))
	}
	if !h.Watching("dev") || h.Watching("stranger") {
		t.Fatal("grant admission mismatch")
	}
	var ended []string
	h.WatchEnded = func(group, user string) { ended = append(ended, group+":"+user) }
	watchGone(h, "g", "by")
	if !h.Watching("dev") || ended != nil {
		t.Fatalf("watching=%v, ended=%v", h.Watching("dev"), ended)
	}
	watchGone(h, "other", "by")
	if h.Watching("dev") || !reflect.DeepEqual(ended, []string{"other:a"}) {
		t.Fatalf("watching=%v, ended=%v", h.Watching("dev"), ended)
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
