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

func TestOpenLogsReceiveAndReload(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, browserKey, laptopKey, otherKey := sesKey("node"), sesKey("browser"), sesKey("laptop"), sesKey("other")
	logs, err := OpenLogs(db, nodeKey.ID())
	if err != nil {
		t.Fatal(err)
	}
	if logs.Owner() != "" || logs.UserOf(browserKey.ID()) != "" || logs.IsOwner(browserKey.ID()) || logs.Known(browserKey.ID()) || logs.Log("") != nil || logs.Log(browserKey.ID()) != nil || logs.frames(browserKey.ID()) != nil {
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
	if logs.Owner() != browserKey.ID() || logs.UserOf(browserKey.ID()) != browserKey.ID() || logs.UserOf(laptopKey.ID()) != "" || !logs.IsOwner(nodeKey.ID()) || logs.IsOwner(laptopKey.ID()) || logs.Log(otherKey.ID()) != nil {
		t.Fatal("owner state mismatch")
	}

	var changeCalls int
	logs.mu.Lock()
	logs.onChange = func(string, []proto.DeviceRow, bool, *sesConn) { changeCalls++ }
	logs.mu.Unlock()

	// Unrelated user, duplicate entries and gaps leave log unchanged without notifying.
	o1 := sesAddEntry(otherKey, otherKey.ID(), 1, otherKey.ID(), "browser", "o")
	if err := logs.dvlReceive(proto.Devlog{Type: "devlog", User: otherKey.ID(), Entries: []proto.Envelope{o1}}, nil); err != nil || logs.Known(otherKey.ID()) || changeCalls != 0 {
		t.Fatalf("other user receive = %v, calls=%d", err, changeCalls)
	}
	if err := logs.dvlReceive(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e1, e2}}, nil); err != nil || changeCalls != 0 {
		t.Fatalf("duplicate receive = %v, calls=%d", err, changeCalls)
	}

	badGap := sesAddEntry(nodeKey, browserKey.ID(), 9, laptopKey.ID(), "node", "gap")
	e3 := sesAddEntry(nodeKey, browserKey.ID(), 3, laptopKey.ID(), "node", "laptop")
	if err := logs.dvlReceive(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{badGap, e3}}, nil); err != nil || changeCalls != 1 || !logs.IsOwner(laptopKey.ID()) {
		t.Fatalf("receive after gap = %v, calls=%d", err, changeCalls)
	}

	revRaw := fmt.Sprintf(`{"t":"device","n":%q,"ts":1,"user":%q,"ver":4,"op":"revoke","dev":%q,"kind":"node","name":"ignored"}`, sesID("rev-extra"), browserKey.ID(), laptopKey.ID())
	revRow := dvlRowOf(nodeKey.Sign(revRaw))
	if revRow.Kind != "" || revRow.Name != "" || revRow.Op != "revoke" || revRow.ID != browserKey.ID()+":4" {
		t.Fatalf("revoke row kept extra fields: %+v", revRow)
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
	ownerJSON := string(dvlMustEncode(browserKey.ID()))
	putOwner := func(db *store.Store) {
		t.Helper()
		put := func(tx *store.Tx) error {
			_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', ?)", ownerJSON)
			return err
		}
		if err := db.Write(put); err != nil {
			t.Fatal(err)
		}
	}

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

	t.Run("unreadable device row", func(t *testing.T) {
		db := sesOpenStore(t)
		putOwner(db)
		corrupt := func(tx *store.Tx) error {
			_, err := tx.Exec("DROP TABLE device_log; CREATE TABLE device_log(user, ver, env); INSERT INTO device_log VALUES (NULL, 1, '{}')")
			return err
		}
		mustNoErr(t, db.Write(corrupt))
		if _, err := OpenLogs(db, nodeKey.ID()); err == nil || !strings.Contains(err.Error(), "converting NULL to string is unsupported") {
			t.Fatalf("OpenLogs error = %v, want unreadable row", err)
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
		putOwner(db)
		logs, err := OpenLogs(db, nodeKey.ID())
		if err != nil || logs.Owner() != browserKey.ID() || logs.IsOwner(browserKey.ID()) || logs.Log(browserKey.ID()) != nil {
			t.Fatalf("logs = %v, %v", logs, err)
		}
		e1 := sesAddEntry(browserKey, browserKey.ID(), 1, browserKey.ID(), "browser", "b")
		if err := logs.dvlReceive(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e1}}, nil); err != nil || !logs.IsOwner(browserKey.ID()) {
			t.Fatalf("receive to empty owner log failed: %v", err)
		}
		reloaded1, err := OpenLogs(db, nodeKey.ID())
		if err != nil || !reloaded1.IsOwner(browserKey.ID()) {
			t.Fatalf("reload 1-entry owner log failed: %v", err)
		}
	})

	t.Run("dropped device_log table", func(t *testing.T) {
		db := sesOpenStore(t)
		putOwner(db)
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
		putOwner(db)
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
		putOwner(db)
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

func TestLogsKeepFirstAtTheSameVersion(t *testing.T) {
	user, device, node := sesKey("kept-user"), sesKey("kept-device"), sesKey("kept-node")
	db := sesOpenStore(t)
	logs := seedOwnedLogs(t, db, node, sesKey("kept-owner"))
	genesis := sesAddEntry(user, user.ID(), 1, user.ID(), "browser", "Original")
	mustNoErr(t, db.Write(func(tx *store.Tx) error { return dvlPutEntries(tx, user.ID(), []proto.DeviceRow{dvlRowOf(genesis)}) })) // as the join stores it
	first, err := identity.Replay(user.ID(), []proto.Envelope{genesis})
	mustNoErr(t, err)
	logs.dvlKeep(first, nil)
	alternate := sesAddEntry(user, user.ID(), 1, user.ID(), "browser", "Alternate")
	sameVersion, err := identity.Replay(user.ID(), []proto.Envelope{alternate})
	mustNoErr(t, err)
	logs.dvlKeep(sameVersion, nil)
	if got := logs.Log(user.ID()).Entries[0]; got != genesis {
		t.Fatalf("kept %v, want first entry %v", got, genesis)
	}
	next, err := identity.Replay(user.ID(), []proto.Envelope{genesis, sesAddEntry(user, user.ID(), 2, device.ID(), "node", "New")})
	mustNoErr(t, err)
	mustNoErr(t, logs.dvlKeep(next, nil))
	if !logs.Known(device.ID()) {
		t.Fatal("longer stored log did not add its device")
	}
	reopened, err := OpenLogs(db, node.ID())
	mustNoErr(t, err)
	mustEqual(t, reopened.Log(user.ID()).Entries, next.Entries) // a rejoin's new entry is stored
}

// A log taken from a join or an import that cannot be stored is reported.
func TestKeptLogWriteFailureIsReported(t *testing.T) {
	ph := newPairHarness(t, nil, false)
	user := sesKey("kept-failure")
	genesis := sesAddEntry(user, user.ID(), 1, user.ID(), "browser", "First")
	first, err := identity.Replay(user.ID(), []proto.Envelope{genesis})
	mustNoErr(t, err)
	mustNoErr(t, ph.logs.dvlKeep(first, nil))
	longer, err := identity.Replay(user.ID(), []proto.Envelope{genesis, sesAddEntry(user, user.ID(), 2, sesKey("kept-failure-node").ID(), "node", "Node")})
	mustNoErr(t, err)
	mustNoErr(t, ph.db.Write(func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE device_log"); return err }))

	loop, err := home.StartLoop(home.LoopConfig{Store: ph.db, Logs: ph.logs.Log})
	mustNoErr(t, err)
	t.Cleanup(loop.Close)
	c := &sesConn{home: loop.Attach(user.ID(), nilClient{}), joining: &joinConn{}}
	ph.pairer.admit(&pairAttempt{joining: map[Ctl]bool{}}, c, longer, false)
	(&nodeRuntime{logs: ph.logs, sessions: ph.sessions}).keepImported(map[string]*identity.Log{user.ID(): longer})
	for range 2 {
		if err := recvFrame(t, ph.reported); err == nil || !strings.Contains(err.Error(), "device_log") {
			t.Fatalf("reported %v, want the failed device_log write", err)
		}
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

	t.Run("receive device_log write failure", func(t *testing.T) {
		db := sesOpenStore(t)
		logs := seedOwnedLogs(t, db, nodeKey, browserKey)
		drop := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE device_log"); return err }
		if err := db.Write(drop); err != nil {
			t.Fatal(err)
		}
		e3 := sesAddEntry(browserKey, browserKey.ID(), 3, sesKey("d3").ID(), "browser", "d3")
		if err := logs.dvlReceive(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e3}}, nil); err == nil {
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
	need := ctlLimit - curSize - 1 - probeSize + 1
	if need < 0 {
		t.Fatalf("unexpected negative padding %d", need)
	}
	exactEntry := sesAddEntry(userKey, user, 51, otherKey.ID(), "node", strings.Repeat("b", need))
	exactFrames := dvlBatchFrames(user, append(append([]proto.Envelope{}, base...), exactEntry))
	if len(exactFrames) != 1 {
		t.Fatalf("at 61,440 bytes got %d frames, want 1", len(exactFrames))
	}
	if got := len(dvlMustEncode(exactFrames[0])); got != ctlLimit {
		t.Fatalf("frame size = %d, want %d", got, ctlLimit)
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

// While the disk is ahead of memory (a join or an import stored the entry), a
// different entry at that version is refused and the same one is taken.
func TestLogsStoredEntryWinsOverAnother(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, owner := sesKey("stored-node"), sesKey("stored-owner")
	logs := seedOwnedLogs(t, db, nodeKey, owner)
	stored := sesAddEntry(owner, owner.ID(), 3, sesKey("stored-a").ID(), "browser", "A")
	other := sesAddEntry(owner, owner.ID(), 3, sesKey("stored-b").ID(), "browser", "B")
	mustNoErr(t, db.Write(func(tx *store.Tx) error { return dvlPutEntries(tx, owner.ID(), []proto.DeviceRow{dvlRowOf(stored)}) }))
	if err := logs.dvlReceive(proto.Devlog{User: owner.ID(), Entries: []proto.Envelope{other}}, nil); err == nil {
		t.Fatal("another entry at a stored version was accepted")
	}
	mustEqual(t, len(logs.Log(owner.ID()).Entries), 2)
	mustNoErr(t, logs.dvlReceive(proto.Devlog{User: owner.ID(), Entries: []proto.Envelope{stored}}, nil))
	reopened, err := OpenLogs(db, nodeKey.ID())
	mustNoErr(t, err)
	mustEqual(t, reopened.Log(owner.ID()).Entries, logs.Log(owner.ID()).Entries)
	mustEqual(t, logs.Log(owner.ID()).Entries[2], stored)
}

// A connection that takes the log gets every frame of a change, in order, and
// none after the first send that fails.
func TestSyncOneSendsEveryFrameUntilASendFails(t *testing.T) {
	nodeKey, owner := sesKey("sync-node"), sesKey("sync-owner")
	s := &Sessions{cfg: SessionConfig{Logs: seedOwnedLogs(t, sesOpenStore(t), nodeKey, owner)}}
	frames := make([]proto.Devlog, 3)
	for i := range frames {
		frames[i] = proto.Devlog{Type: "devlog", User: owner.ID()}
	}
	whole, broken := newFakeCtl(), newFakeCtl()
	broken.failAfter = 2
	for _, raw := range []*fakeCtl{whole, broken} {
		s.sesSyncOne(&sesConn{welcomed: true, raw: raw}, owner.ID(), frames, nil)
	}
	mustEqual(t, whole.sends, 3)
	mustEqual(t, broken.sends, 2)
}
