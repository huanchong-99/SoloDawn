package node

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func sesKey(name string) identity.Key {
	sum := sha256.Sum256([]byte("node-session-key:" + name))
	return identity.Key(ed25519.NewKeyFromSeed(sum[:]))
}

func sesID(name string) proto.ID {
	sum := sha256.Sum256([]byte("node-session-id:" + name))
	return proto.ID(base64.RawURLEncoding.EncodeToString(sum[:16]))
}

func sesAddEntry(signer identity.Key, user string, ver int, dev, kind, name string) proto.Envelope {
	p, err := json.Marshal(proto.DeviceEntry{
		PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: sesID(fmt.Sprintf("add-%s-%d", dev, ver)), Timestamp: 1763600000000},
		DeviceChange:  proto.DeviceChange{User: user, Version: ver, Op: "add", Device: dev, Kind: kind, Name: name},
	})
	if err != nil {
		panic(err)
	}
	return signer.Sign(string(p))
}

func sesRevokeEntry(signer identity.Key, user string, ver int, dev string) proto.Envelope {
	p, err := json.Marshal(proto.DeviceEntry{
		PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: sesID(fmt.Sprintf("rev-%s-%d", dev, ver)), Timestamp: 1763600000000},
		DeviceChange:  proto.DeviceChange{User: user, Version: ver, Op: "revoke", Device: dev},
	})
	if err != nil {
		panic(err)
	}
	return signer.Sign(string(p))
}

func sesOpenStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenLogsAndReload(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, browserKey, laptopKey := sesKey("node"), sesKey("browser"), sesKey("laptop")
	logs, err := OpenLogs(db, nodeKey.ID())
	if err != nil {
		t.Fatal(err)
	}
	if logs.Owner() != "" || logs.IsOwner(browserKey.ID()) || logs.Known(browserKey.ID()) || logs.Log(browserKey.ID()) != nil || logs.Frames(browserKey.ID()) != nil {
		t.Fatal("empty logs reported state")
	}

	e1 := sesAddEntry(browserKey, browserKey.ID(), 1, browserKey.ID(), "browser", "phone")
	e2 := sesAddEntry(browserKey, browserKey.ID(), 2, nodeKey.ID(), "node", "workstation")
	cand, err := identity.Replay(browserKey.ID(), []proto.Envelope{e1, e2})
	if err != nil {
		t.Fatal(err)
	}
	adopted, err := logs.dvlAdopt(browserKey.ID(), cand, nil)
	if err != nil || !adopted {
		t.Fatalf("adopt = %v, %v", adopted, err)
	}
	if second, err := logs.dvlAdopt(browserKey.ID(), cand, nil); err != nil || second {
		t.Fatalf("second adopt = %v, %v", second, err)
	}
	if logs.Owner() != browserKey.ID() || !logs.IsOwner(browserKey.ID()) || !logs.IsOwner(nodeKey.ID()) || logs.IsOwner(laptopKey.ID()) {
		t.Fatal("owner state mismatch")
	}

	e3 := sesAddEntry(nodeKey, browserKey.ID(), 3, laptopKey.ID(), "node", "laptop")
	var changeCalls int
	logs.mu.Lock()
	logs.onChange = func(string, []proto.DeviceRow, bool, *sesConn) { changeCalls++ }
	logs.mu.Unlock()
	row, err := logs.Append(e3)
	if err != nil || row.ID != browserKey.ID()+":3" || row.Kind != "node" || row.Name != "laptop" || changeCalls != 1 {
		t.Fatalf("append = %+v, %v, calls=%d", row, err, changeCalls)
	}
	dupRow, err := logs.Append(e3)
	if err != nil || dupRow.ID != row.ID || changeCalls != 1 {
		t.Fatalf("duplicate append = %+v, %v, calls=%d", dupRow, err, changeCalls)
	}

	reloaded, err := OpenLogs(db, nodeKey.ID())
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Owner() != browserKey.ID() || !reloaded.IsOwner(laptopKey.ID()) {
		t.Fatal("reloaded owner state mismatch")
	}
	copied := reloaded.Log(browserKey.ID())
	if copied == nil || len(copied.Entries) != 3 {
		t.Fatalf("copied log = %+v", copied)
	}
}

func TestOpenLogsErrors(t *testing.T) {
	nodeKey, browserKey := sesKey("node"), sesKey("browser")

	t.Run("dropped setting table", func(t *testing.T) {
		db := sesOpenStore(t)
		drop := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE setting"); return err }
		if err := db.Write(drop); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenLogs(db, nodeKey.ID()); err == nil {
			t.Fatal("expected error when setting table is missing")
		}
	})

	t.Run("invalid owner json", func(t *testing.T) {
		db := sesOpenStore(t)
		corrupt := func(tx *store.Tx) error {
			_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', 'not-json')")
			return err
		}
		if err := db.Write(corrupt); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenLogs(db, nodeKey.ID()); err == nil {
			t.Fatal("expected error for invalid owner json")
		}
	})

	t.Run("owner set before entries", func(t *testing.T) {
		db := sesOpenStore(t)
		ownerJSON := string(dvlMustEncode(browserKey.ID()))
		put := func(tx *store.Tx) error {
			_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', ?)", ownerJSON)
			return err
		}
		if err := db.Write(put); err != nil {
			t.Fatal(err)
		}
		logs, err := OpenLogs(db, nodeKey.ID())
		if err != nil || logs.Owner() != browserKey.ID() || logs.IsOwner(browserKey.ID()) {
			t.Fatalf("logs = %v, %v", logs, err)
		}
		if _, err := logs.Append(sesAddEntry(browserKey, browserKey.ID(), 1, browserKey.ID(), "browser", "b")); err != nil {
			t.Fatalf("append to kept owner failed: %v", err)
		}
	})

	t.Run("dropped device_log table", func(t *testing.T) {
		db := sesOpenStore(t)
		drop := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE device_log"); return err }
		if err := db.Write(drop); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenLogs(db, nodeKey.ID()); err == nil {
			t.Fatal("expected error when device_log table is missing")
		}
	})

	t.Run("corrupt device_log envelope", func(t *testing.T) {
		db := sesOpenStore(t)
		corrupt := func(tx *store.Tx) error {
			_, err := tx.Exec("INSERT INTO device_log(user, ver, env) VALUES(?, 1, '{}')", browserKey.ID())
			return err
		}
		if err := db.Write(corrupt); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenLogs(db, nodeKey.ID()); err != identity.BadRequest {
			t.Fatalf("got %v, want bad_request", err)
		}
	})

	t.Run("invalid device_log signature", func(t *testing.T) {
		db := sesOpenStore(t)
		bad := sesAddEntry(nodeKey, browserKey.ID(), 1, browserKey.ID(), "browser", "b")
		raw := string(dvlMustEncode(bad))
		put := func(tx *store.Tx) error {
			_, err := tx.Exec("INSERT INTO device_log(user, ver, env) VALUES(?, 1, ?)", browserKey.ID(), raw)
			return err
		}
		if err := db.Write(put); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenLogs(db, nodeKey.ID()); err == nil {
			t.Fatal("expected replay error")
		}
	})
}

func TestLogsAppendReceiveKeepAndUserOrder(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey := sesKey("node")
	logs, err := OpenLogs(db, nodeKey.ID())
	if err != nil {
		t.Fatal(err)
	}

	logs.Keep("not-a-device")
	u1, u2, shared := sesKey("u1"), sesKey("u2"), sesKey("shared")
	if u1.ID() > u2.ID() {
		u1, u2 = u2, u1
	}
	logs.Keep(u2.ID())

	if _, err := logs.Append(proto.Envelope{Payload: `{"user":123}`}); err != identity.BadRequest {
		t.Fatalf("bad payload = %v", err)
	}
	if _, err := logs.Append(sesAddEntry(u1, "invalid", 1, u1.ID(), "browser", "b")); err != identity.BadRequest {
		t.Fatalf("invalid user = %v", err)
	}
	if _, err := logs.Append(sesAddEntry(u1, u1.ID(), 1, u1.ID(), "browser", "b")); err != identity.UnknownDevice {
		t.Fatalf("unkept user = %v", err)
	}
	if _, err := logs.Append(sesAddEntry(u2, u2.ID(), 2, u2.ID(), "browser", "b")); err == nil {
		t.Fatal("expected gap error")
	}

	if err := logs.Receive(proto.Devlog{Type: "devlog", User: "bad", Entries: []proto.Envelope{{}}}); err != nil {
		t.Fatal(err)
	}
	if err := logs.Receive(proto.Devlog{Type: "devlog", User: u2.ID()}); err != nil {
		t.Fatal(err)
	}
	if err := logs.Receive(proto.Devlog{Type: "devlog", User: u1.ID(), Entries: []proto.Envelope{sesAddEntry(u1, u1.ID(), 1, u1.ID(), "browser", "b")}}); err != nil {
		t.Fatal(err)
	}
	if logs.Known(u1.ID()) {
		t.Fatal("unkept user should be ignored")
	}

	badGap := sesAddEntry(u2, u2.ID(), 5, shared.ID(), "browser", "s")
	g2 := sesAddEntry(u2, u2.ID(), 1, u2.ID(), "browser", "b2")
	a2 := sesAddEntry(u2, u2.ID(), 2, shared.ID(), "browser", "shared")
	if err := logs.Receive(proto.Devlog{Type: "devlog", User: u2.ID(), Entries: []proto.Envelope{badGap, g2, a2}}); err != nil {
		t.Fatal(err)
	}
	if err := logs.Receive(proto.Devlog{Type: "devlog", User: u2.ID(), Entries: []proto.Envelope{g2, a2}}); err != nil {
		t.Fatal(err)
	}

	logs.Keep(u1.ID())
	g1 := sesAddEntry(u1, u1.ID(), 1, u1.ID(), "browser", "b1")
	a1 := sesAddEntry(u1, u1.ID(), 2, shared.ID(), "browser", "shared")
	if err := logs.Receive(proto.Devlog{Type: "devlog", User: u1.ID(), Entries: []proto.Envelope{g1, a1}}); err != nil {
		t.Fatal(err)
	}
	if got := logs.UserOf(shared.ID()); got != u1.ID() {
		t.Fatalf("UserOf(shared) = %s, want lower %s", got, u1.ID())
	}

	revRaw := fmt.Sprintf(`{"t":"device","n":%q,"ts":1,"user":%q,"ver":3,"op":"revoke","dev":%q,"kind":"browser","name":"ignored"}`, sesID("rev-extra"), u1.ID(), shared.ID())
	revRow, err := logs.Append(u1.Sign(revRaw))
	if err != nil {
		t.Fatal(err)
	}
	if revRow.Kind != "" || revRow.Name != "" || revRow.Op != "revoke" {
		t.Fatalf("revoke row kept extra fields: %+v", revRow)
	}
}

func TestLogsStoreWriteFailures(t *testing.T) {
	nodeKey, browserKey := sesKey("node"), sesKey("browser")
	e1 := sesAddEntry(browserKey, browserKey.ID(), 1, browserKey.ID(), "browser", "b")
	e2 := sesAddEntry(browserKey, browserKey.ID(), 2, nodeKey.ID(), "node", "n")

	t.Run("adoption setting write failure", func(t *testing.T) {
		db := sesOpenStore(t)
		logs, err := OpenLogs(db, nodeKey.ID())
		if err != nil {
			t.Fatal(err)
		}
		drop := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE setting"); return err }
		if err := db.Write(drop); err != nil {
			t.Fatal(err)
		}
		cand, err := identity.Replay(browserKey.ID(), []proto.Envelope{e1, e2})
		if err != nil {
			t.Fatal(err)
		}
		if adopted, err := logs.dvlAdopt(browserKey.ID(), cand, nil); err == nil || adopted {
			t.Fatalf("adopt = %v, %v", adopted, err)
		}
	})

	t.Run("adoption device_log write failure", func(t *testing.T) {
		db := sesOpenStore(t)
		logs, err := OpenLogs(db, nodeKey.ID())
		if err != nil {
			t.Fatal(err)
		}
		drop := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE device_log"); return err }
		if err := db.Write(drop); err != nil {
			t.Fatal(err)
		}
		cand, err := identity.Replay(browserKey.ID(), []proto.Envelope{e1, e2})
		if err != nil {
			t.Fatal(err)
		}
		if adopted, err := logs.dvlAdopt(browserKey.ID(), cand, nil); err == nil || adopted {
			t.Fatalf("adopt = %v, %v", adopted, err)
		}
	})

	t.Run("append device_log write failure", func(t *testing.T) {
		db := sesOpenStore(t)
		logs, err := OpenLogs(db, nodeKey.ID())
		if err != nil {
			t.Fatal(err)
		}
		logs.Keep(browserKey.ID())
		drop := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE device_log"); return err }
		if err := db.Write(drop); err != nil {
			t.Fatal(err)
		}
		if _, err := logs.Append(e1); err == nil {
			t.Fatal("expected error when device_log table is missing")
		}
	})
}

func TestBatchFramesExact60KiBBoundary(t *testing.T) {
	userKey, otherKey := sesKey("boundary-user"), sesKey("boundary-other")
	user := userKey.ID()

	var base []proto.Envelope
	pad := strings.Repeat("a", 800)
	for i := 1; i <= 50; i++ {
		dev := otherKey.ID()
		if i == 1 {
			dev = user
		}
		base = append(base, sesAddEntry(userKey, user, i, dev, "node", pad))
	}
	one := dvlBatchFrames(user, base)
	if len(one) != 1 {
		t.Fatalf("base frames = %d, want 1", len(one))
	}
	curSize := len(dvlMustEncode(one[0]))
	probe := sesAddEntry(userKey, user, 51, otherKey.ID(), "node", "b")
	probeSize := len(dvlMustEncode(probe))
	need := dvlCtlLimit - curSize - 1 - probeSize + 1
	if need < 0 {
		t.Fatalf("unexpected negative padding %d", need)
	}
	exactEntry := sesAddEntry(userKey, user, 51, otherKey.ID(), "node", strings.Repeat("b", need))
	exactFrames := dvlBatchFrames(user, append(append([]proto.Envelope{}, base...), exactEntry))
	if len(exactFrames) != 1 {
		t.Fatalf("at 61,440 bytes got %d frames, want 1", len(exactFrames))
	}
	if got := len(dvlMustEncode(exactFrames[0])); got != dvlCtlLimit {
		t.Fatalf("frame size = %d, want %d", got, dvlCtlLimit)
	}

	overEntry := sesAddEntry(userKey, user, 51, otherKey.ID(), "node", strings.Repeat("b", need+1))
	splitFrames := dvlBatchFrames(user, append(append([]proto.Envelope{}, base...), overEntry))
	if len(splitFrames) != 2 || len(splitFrames[0].Entries) != 50 || len(splitFrames[1].Entries) != 1 {
		t.Fatalf("at 61,441 bytes got %d frames, want 2 (50 + 1)", len(splitFrames))
	}
	if strings.Contains(string(dvlMustEncode(map[string]string{"x": "<&>"})), `\u003c`) {
		t.Fatal("HTML characters were escaped")
	}
}

func TestHomeDeviceEvent(t *testing.T) {
	ownerKey, memberKey, strangerKey, devKey := sesKey("owner"), sesKey("member"), sesKey("stranger"), sesKey("dev2")
	s := home.NewState("g1", sesKey("home").ID(), ownerKey.ID())
	s.Group = proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: "g1"}, Name: "Website"}
	s.People["m-member"] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "m-member"}, User: memberKey.ID()},
		Role:      "member",
	}

	if res := dvlHomeEvent(s, nil, 0, "wrong"); len(res.Rows) != 0 {
		t.Fatal("expected empty result for wrong event type")
	}
	if res := dvlHomeEvent(s, nil, 0, []proto.DeviceRow{}); len(res.Rows) != 0 {
		t.Fatal("expected empty result for empty slice")
	}
	strangerRow := dvlRowOf(sesAddEntry(strangerKey, strangerKey.ID(), 1, strangerKey.ID(), "browser", "s"))
	if res := dvlHomeEvent(s, nil, 0, []proto.DeviceRow{strangerRow}); len(res.Rows) != 0 {
		t.Fatal("expected empty result for unrelated user")
	}

	r1 := dvlRowOf(sesAddEntry(ownerKey, ownerKey.ID(), 1, ownerKey.ID(), "browser", "b"))
	res := dvlHomeEvent(s, nil, 0, []proto.DeviceRow{r1})
	if len(res.Rows) != 1 || res.GroupsChanged {
		t.Fatalf("offline add result = %+v", res)
	}
	s.Commit(s.Seal(res))

	s.Online[devKey.ID()] = true
	r2 := dvlRowOf(sesAddEntry(ownerKey, ownerKey.ID(), 2, devKey.ID(), "browser", "b2"))
	res = dvlHomeEvent(s, nil, 0, []proto.DeviceRow{r1, r2})
	if len(res.Rows) != 1 || !res.GroupsChanged {
		t.Fatalf("online add result = %+v", res)
	}
	s.Commit(s.Seal(res))

	r3 := dvlRowOf(sesAddEntry(ownerKey, ownerKey.ID(), 3, devKey.ID(), "browser", "renamed"))
	res = dvlHomeEvent(s, nil, 0, []proto.DeviceRow{r3})
	if len(res.Rows) != 1 || res.GroupsChanged {
		t.Fatalf("online rename result = %+v", res)
	}
	s.Commit(s.Seal(res))

	neverKey := sesKey("never")
	s.Online[neverKey.ID()] = true
	r4 := dvlRowOf(sesRevokeEntry(ownerKey, ownerKey.ID(), 4, neverKey.ID()))
	res = dvlHomeEvent(s, nil, 0, []proto.DeviceRow{r4})
	if len(res.Rows) != 1 || res.GroupsChanged {
		t.Fatalf("revoke never-added result = %+v", res)
	}
	s.Commit(s.Seal(res))

	r5 := dvlRowOf(sesRevokeEntry(ownerKey, ownerKey.ID(), 5, devKey.ID()))
	res = dvlHomeEvent(s, nil, 0, []proto.DeviceRow{r5})
	if len(res.Rows) != 1 || !res.GroupsChanged {
		t.Fatalf("online revoke result = %+v", res)
	}

	mRow := dvlRowOf(sesAddEntry(memberKey, memberKey.ID(), 1, memberKey.ID(), "browser", "m"))
	if mRes := dvlHomeEvent(s, nil, 0, []proto.DeviceRow{mRow}); len(mRes.Rows) != 1 {
		t.Fatalf("member row result = %+v", mRes)
	}
}
