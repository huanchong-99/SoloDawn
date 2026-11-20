package host

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const seamStart = 1_763_600_000_000 // the first reading of a test clock, in Unix milliseconds

var (
	seamTask   = SessionKey{Group: "g1", Member: "m7", Task: "t12"}
	seamGroup  = SessionKey{Group: "g1", Member: "m7"}
	seamReview = SessionKey{Group: "g1", Member: "m7", Task: "t9"}
)

// seamClock reads one second later at every call.
func seamClock() func() time.Time {
	at := int64(seamStart - 1000)
	return func() time.Time {
		at += 1000
		return time.UnixMilli(at)
	}
}

func seamKey() identity.Key {
	return identity.Key(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize)))
}

func seamStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// seamHost starts a host on db, as a node does, and returns it with the
// sessions the run before left.
func seamHost(t *testing.T, db *store.Store) (*Host, []Session) {
	t.Helper()
	h, saved, err := New(db, seamKey(), seamClock())
	if err != nil {
		t.Fatal(err)
	}
	return h, saved
}

func seamSQL(t *testing.T, db *store.Store, statements ...string) {
	t.Helper()
	for _, statement := range statements {
		if err := db.Write(func(tx *store.Tx) error { _, err := tx.Exec(statement); return err }); err != nil {
			t.Fatal(err)
		}
	}
}

func seamSession(key SessionKey) Session {
	return Session{SessionKey: key, Kind: TaskSession, Folder: "wt/12-paths", Agent: "claude", Home: "home-device", Name: "Claude Code"}
}

// seamCode fails the test unless err is an *Error with that code.
func seamCode(t *testing.T, err error, code string) {
	t.Helper()
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != code {
		t.Fatalf("got %v, want %s", err, code)
	}
}

// seamPaths leads a new session to each state through listed changes only.
var seamPaths = map[State][]State{
	Dormant:  {},
	Working:  {Working},
	Waiting:  {Working, Waiting},
	Takeover: {Takeover},
}

// seamIn makes a session for key and leads it to state.
func seamIn(t *testing.T, sessions *Sessions, key SessionKey, state State) {
	t.Helper()
	if err := sessions.Create(seamSession(key)); err != nil {
		t.Fatal(err)
	}
	at := Dormant
	for _, next := range seamPaths[state] {
		if err := sessions.Move(key, at, next); err != nil {
			t.Fatal(err)
		}
		at = next
	}
}

func seamState(t *testing.T, sessions *Sessions, key SessionKey) State {
	t.Helper()
	s, ok := sessions.Get(key)
	if !ok {
		t.Fatalf("no session %v", key)
	}
	return s.State
}

// Every change of sessions.md section 2 that N makes while it runs, row by row.
func TestSessionChangesOfTheContract(t *testing.T) {
	rows := []struct {
		when     string
		from, to State
	}{
		{"N starts the CLI for an item or a turn of its own", Dormant, Working},
		{"N writes a turn to the open process", Waiting, Working},
		{"the turn ends", Working, Waiting},
		{"idle, the session cap, or all AIs offline", Waiting, Dormant},
		{"the CLI exits during a turn, or all AIs offline", Working, Dormant},
		{"takeover.start is accepted, dormant", Dormant, Takeover},
		{"takeover.start is accepted, waiting", Waiting, Takeover},
		{"takeover.start is accepted, working", Working, Takeover},
		{"hand-back", Takeover, Working},
		{"the interactive CLI cannot start", Takeover, Dormant},
	}
	for _, row := range rows {
		t.Run(row.when, func(t *testing.T) {
			db := seamStore(t)
			h, _ := seamHost(t, db)
			seamIn(t, h.Sessions, seamTask, row.from)
			before, _ := h.Sessions.Get(seamTask)
			if err := h.Sessions.Move(seamTask, row.from, row.to); err != nil {
				t.Fatal(err)
			}
			after, _ := h.Sessions.Get(seamTask)
			if after.State != row.to || after.LastActive != before.LastActive+1000 {
				t.Fatalf("got %s at %d, want %s one second after %d", after.State, after.LastActive, row.to, before.LastActive)
			}
			if _, saved := seamHost(t, db); len(saved) != 1 || saved[0] != after {
				t.Fatalf("stored %+v, want %+v", saved, after)
			}
		})
	}
	if len(rows) != len(changes[Dormant])+len(changes[Waiting])+len(changes[Working])+len(changes[Takeover]) {
		t.Fatal("the rows above and the table of changes differ")
	}
}

// The last two rows of sessions.md section 2: what the start of the node does.
func TestSessionChangesWhenTheNodeStarts(t *testing.T) {
	db := seamStore(t)
	h, saved := seamHost(t, db)
	if len(saved) != 0 || len(h.Sessions.All()) != 0 {
		t.Fatalf("a new store holds %v", saved)
	}
	other := SessionKey{Group: "g1", Member: "m8"}
	asleep := SessionKey{Group: "g2", Member: "m7"}
	taken := SessionKey{Group: "g1", Member: "m9", Task: "t12"}
	seamIn(t, h.Sessions, seamTask, Working)
	seamIn(t, h.Sessions, other, Waiting)
	seamIn(t, h.Sessions, asleep, Dormant)
	seamIn(t, h.Sessions, taken, Takeover)
	left := h.Sessions.All()

	h, saved = seamHost(t, db)
	if !reflect.DeepEqual(saved, left) {
		t.Fatalf("saved %+v, want the rows as the last run left them: %+v", saved, left)
	}
	want := map[SessionKey]State{seamTask: Dormant, other: Dormant, asleep: Dormant, taken: Takeover}
	for key, state := range want {
		if got := seamState(t, h.Sessions, key); got != state {
			t.Errorf("%v is %s after the start, want %s", key, got, state)
		}
	}
	// The table is stored as it is in memory: a third start finds it so.
	if _, again := seamHost(t, db); !reflect.DeepEqual(again, h.Sessions.All()) {
		t.Fatalf("stored %+v, want %+v", again, h.Sessions.All())
	}
	// A session saved as takeover is handed back: the structured CLI starts
	// with the hand-back turn (flows/takeover.md E).
	if err := h.Sessions.Move(taken, Takeover, Working); err != nil {
		t.Fatal(err)
	}
	// The session that was working when the node stopped starts again from
	// dormant, with its row unchanged otherwise.
	if err := h.Sessions.Move(seamTask, Dormant, Working); err != nil {
		t.Fatal(err)
	}
}

func TestSessionChangesTheContractDoesNotList(t *testing.T) {
	states := []State{Dormant, Working, Waiting, Takeover}
	refused := 0
	for _, from := range states {
		for _, to := range states {
			h, _ := seamHost(t, seamStore(t))
			seamIn(t, h.Sessions, seamTask, from)
			before, _ := h.Sessions.Get(seamTask)
			// The session is not in the state the caller takes it to be in.
			for _, wrong := range states {
				if wrong != from {
					seamCode(t, h.Sessions.Move(seamTask, wrong, to), "conflict")
				}
			}
			err := h.Sessions.Move(seamTask, from, to)
			if (from == Dormant && (to == Working || to == Takeover)) ||
				(from == Waiting && to != Waiting) || (from == Working && to != Working) ||
				(from == Takeover && (to == Working || to == Dormant)) {
				if err != nil {
					t.Fatalf("%s to %s: %v", from, to, err)
				}
				continue
			}
			refused++
			seamCode(t, err, "conflict")
			if want := "conflict: the session is " + string(from) + ": no change from " + string(from) + " to " + string(to); err.Error() != want {
				t.Fatalf("got %q, want %q", err, want)
			}
			if after, _ := h.Sessions.Get(seamTask); after != before {
				t.Fatalf("a refused change left %+v, want %+v", after, before)
			}
		}
	}
	if refused != 6 {
		t.Fatalf("%d changes were refused, want 6", refused)
	}
	h, _ := seamHost(t, seamStore(t))
	seamCode(t, h.Sessions.Move(seamTask, Dormant, Working), "not_found")
	if _, ok := h.Sessions.Get(seamTask); ok {
		t.Fatal("a session that was never made is there")
	}
}

// An AI runs one turn at a time per group, and a takeover holds its other
// sessions (sessions.md section 1, flows/takeover.md step 9).
func TestOneActiveSessionPerAIAndGroup(t *testing.T) {
	for _, held := range []State{Working, Takeover} {
		for _, to := range []State{Working, Takeover} {
			h, _ := seamHost(t, seamStore(t))
			seamIn(t, h.Sessions, seamTask, held)
			seamIn(t, h.Sessions, seamGroup, Dormant)
			err := h.Sessions.Move(seamGroup, Dormant, to)
			seamCode(t, err, "busy")
			if want := "busy: another session of this AI in the group is working or taken over"; err.Error() != want {
				t.Fatalf("got %q, want %q", err, want)
			}
			if seamState(t, h.Sessions, seamGroup) != Dormant {
				t.Fatal("a refused change was made")
			}
			// The same AI in another group, and another AI in this group, are free.
			for _, free := range []SessionKey{{Group: "g2", Member: "m7", Task: "t12"}, {Group: "g1", Member: "m8", Task: "t12"}} {
				seamIn(t, h.Sessions, free, Dormant)
				if err := h.Sessions.Move(free, Dormant, to); err != nil {
					t.Fatalf("%v: %v", free, err)
				}
			}
			// The session that holds the AI is not in its own way.
			if err := h.Sessions.Move(seamTask, held, Dormant); err != nil {
				t.Fatal(err)
			}
			if err := h.Sessions.Move(seamGroup, Dormant, to); err != nil {
				t.Fatal(err)
			}
		}
	}
	h, _ := seamHost(t, seamStore(t))
	seamIn(t, h.Sessions, seamGroup, Waiting)
	seamIn(t, h.Sessions, seamTask, Takeover)
	seamIn(t, h.Sessions, seamReview, Dormant)
	// A session that is open or asleep holds nobody, and leaving a turn is
	// never refused.
	if err := h.Sessions.Move(seamGroup, Waiting, Dormant); err != nil {
		t.Fatal(err)
	}
	if err := h.Sessions.Move(seamTask, Takeover, Working); err != nil {
		t.Fatal(err)
	}
	if err := h.Sessions.Move(seamTask, Working, Waiting); err != nil {
		t.Fatal(err)
	}
	if err := h.Sessions.Move(seamReview, Dormant, Working); err != nil {
		t.Fatal(err)
	}
}

func TestSessionTable(t *testing.T) {
	db := seamStore(t)
	h, _ := seamHost(t, db)
	full := Session{SessionKey: seamTask, Kind: TaskSession, Folder: "wt/12-paths", Agent: "codex", Home: "home-device",
		Name: "Codex", Secretary: true, CLISession: "thread-1", State: Takeover, LastActive: 5, TakeoverAt: "9f2c1ab"}
	if err := h.Sessions.Create(full); err != nil {
		t.Fatal(err)
	}
	// A new session is dormant from now on, whatever the caller filled in.
	full.State, full.LastActive = Dormant, seamStart
	if got, ok := h.Sessions.Get(seamTask); !ok || got != full {
		t.Fatalf("got %+v, want %+v", got, full)
	}
	seamCode(t, h.Sessions.Create(seamSession(seamTask)), "conflict")
	if got, _ := h.Sessions.Get(seamTask); got != full {
		t.Fatalf("a refused session replaced the first: %+v", got)
	}

	keys := []SessionKey{{Group: "g2", Member: "m1"}, {Group: "g1", Member: "m8"}, seamReview, seamGroup}
	for _, key := range keys {
		plain := Session{SessionKey: key, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: "home-device", Name: "Claude Code"}
		if err := h.Sessions.Create(plain); err != nil {
			t.Fatal(err)
		}
	}
	var order []SessionKey
	for _, s := range h.Sessions.All() {
		order = append(order, s.SessionKey)
	}
	if want := []SessionKey{seamGroup, seamTask, seamReview, {Group: "g1", Member: "m8"}, {Group: "g2", Member: "m1"}}; !reflect.DeepEqual(order, want) {
		t.Fatalf("got %v, want %v", order, want)
	}
	// Every column comes back, the empty ones as they were.
	if _, saved := seamHost(t, db); !reflect.DeepEqual(saved, h.Sessions.All()) || saved[1] != full || saved[0].CLISession != "" || saved[0].Secretary {
		t.Fatalf("stored %+v, want %+v", saved, h.Sessions.All())
	}

	// Update writes what its caller changed and nothing else.
	err := h.Sessions.Update(seamGroup, func(s *Session) {
		s.CLISession, s.TakeoverAt, s.Secretary = "8d1f", "c0ffee1", true
	})
	if err != nil {
		t.Fatal(err)
	}
	changed, _ := h.Sessions.Get(seamGroup)
	want := Session{SessionKey: seamGroup, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: "home-device", Name: "Claude Code",
		Secretary: true, CLISession: "8d1f", State: Dormant, LastActive: seamStart + 4000, TakeoverAt: "c0ffee1"}
	if changed != want {
		t.Fatalf("got %+v, want %+v", changed, want)
	}
	if got, _ := h.Sessions.Get(seamTask); got != full {
		t.Fatalf("an update of one session changed another: %+v", got)
	}
	seamCode(t, h.Sessions.Update(SessionKey{Group: "g9"}, func(*Session) { t.Error("no session to change") }), "not_found")

	if err := h.Sessions.End(seamGroup); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.Sessions.Get(seamGroup); ok || len(h.Sessions.All()) != 4 {
		t.Fatal("an ended session is still there")
	}
	if _, saved := seamHost(t, db); len(saved) != 4 || saved[0] != full {
		t.Fatalf("stored %+v after the end of one session", saved)
	}
	// A session that is not there has ended already.
	if err := h.Sessions.End(seamGroup); err != nil || len(h.Sessions.All()) != 4 {
		t.Fatalf("got %v and %d sessions", err, len(h.Sessions.All()))
	}
}

func TestSessionTableKeepsMemoryAndStoreAlike(t *testing.T) {
	db := seamStore(t)
	h, _ := seamHost(t, db)
	seamIn(t, h.Sessions, seamTask, Working)
	before, _ := h.Sessions.Get(seamTask)
	seamSQL(t, db,
		"CREATE TRIGGER no_write BEFORE INSERT ON session BEGIN SELECT RAISE(ABORT, 'the test refuses the write'); END",
		"CREATE TRIGGER no_delete BEFORE DELETE ON session BEGIN SELECT RAISE(ABORT, 'the test refuses the delete'); END")
	failures := map[string]error{
		"create": h.Sessions.Create(seamSession(seamGroup)),
		"move":   h.Sessions.Move(seamTask, Working, Waiting),
		"update": h.Sessions.Update(seamTask, func(s *Session) { s.CLISession = "lost" }),
	}
	for name, err := range failures {
		if err == nil || !strings.Contains(err.Error(), "the test refuses the write") {
			t.Errorf("%s: got %v, want the store's error", name, err)
		}
	}
	if err := h.Sessions.End(seamTask); err == nil || !strings.Contains(err.Error(), "the test refuses the delete") {
		t.Fatalf("got %v, want the store's error", err)
	}
	if got := h.Sessions.All(); len(got) != 1 || got[0] != before {
		t.Fatalf("after refused writes the table holds %+v, want %+v", got, before)
	}
}

func TestSessionTableThatCannotBeLoaded(t *testing.T) {
	columns := "(g, member, task, kind, folder, agent, home, name, secretary, cli_session, state, last_active, takeover_at)"
	cases := map[string][]string{
		"no such table": {"DROP TABLE session"},
		"the test refuses the update": {
			"INSERT INTO session" + columns + " VALUES('g1', 'm7', '', 'group', 'repo', 'claude', 'home', 'Claude Code', 0, NULL, 'working', 1, NULL)",
			"CREATE TRIGGER no_update BEFORE UPDATE ON session BEGIN SELECT RAISE(ABORT, 'the test refuses the update'); END",
		},
		"cannot unmarshal string": {
			"DROP TABLE session", "CREATE TABLE session " + columns,
			"INSERT INTO session VALUES('g1', 'm7', '', 'group', 'repo', 'claude', 'home', 'Claude Code', 0, NULL, 'dormant', 'soon', NULL)",
		},
	}
	for want, statements := range cases {
		db := seamStore(t)
		seamSQL(t, db, statements...)
		h, saved, err := New(db, seamKey(), seamClock())
		if err == nil || !strings.Contains(err.Error(), want) || h != nil || saved != nil {
			t.Errorf("got %v, %v and %v, want no host and an error with %q", h, saved, err, want)
		}
	}
}

func TestSessionTokens(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seamIn(t, h.Sessions, seamTask, Dormant)
	seamIn(t, h.Sessions, seamGroup, Dormant)
	structured := h.Sessions.Admit(seamTask, 2, false)
	terminal := h.Sessions.Admit(seamTask, 3, true)
	other := h.Sessions.Admit(seamGroup, 1, false)
	if raw, err := base64.RawURLEncoding.DecodeString(structured); err != nil || len(raw) != 16 || structured == terminal {
		t.Fatalf("tokens %q and %q, want two different tokens of 16 random bytes", structured, terminal)
	}
	if err := h.Sessions.Move(seamTask, Dormant, Working); err != nil {
		t.Fatal(err)
	}
	now, _ := h.Sessions.Get(seamTask)
	if c, ok := h.Sessions.Caller(structured); !ok || c != (Caller{Session: now, Level: 2}) {
		t.Fatalf("got %+v, want the session as it is now at level 2", c)
	}
	if c, ok := h.Sessions.Caller(terminal); !ok || c != (Caller{Session: now, Level: 3, Interactive: true}) {
		t.Fatalf("got %+v, want the session in a terminal at level 3", c)
	}
	if _, ok := h.Sessions.Caller("never issued"); ok {
		t.Fatal("an unknown token names a caller")
	}
	h.Sessions.Forget(structured)
	if _, ok := h.Sessions.Caller(structured); ok {
		t.Fatal("a forgotten token names a caller")
	}
	if _, ok := h.Sessions.Caller(terminal); !ok {
		t.Fatal("forgetting one token dropped another")
	}
	if err := h.Sessions.End(seamTask); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.Sessions.Caller(terminal); ok {
		t.Fatal("the token of an ended session names a caller")
	}
	// The process of the old session has no part in one made later for the
	// same key, and the end of one session leaves the tokens of the others.
	seamIn(t, h.Sessions, seamTask, Dormant)
	if c, ok := h.Sessions.Caller(terminal); ok {
		t.Fatalf("the token of an ended session names the session made after it: %+v", c)
	}
	if c, ok := h.Sessions.Caller(other); !ok || c.SessionKey != seamGroup || c.Level != 1 {
		t.Fatalf("got %+v, want the group session, whose token was not touched", c)
	}
}

// The tools of a turn go by what the host noted about it.
func TestCallerCarriesTheTurn(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seamIn(t, h.Sessions, seamTask, Dormant)
	seamIn(t, h.Sessions, seamGroup, Dormant)
	token, other := h.Sessions.Admit(seamTask, 2, false), h.Sessions.Admit(seamGroup, 1, false)
	if c, _ := h.Sessions.Caller(token); c.Turn != (Turn{}) {
		t.Fatalf("got %+v before any turn", c.Turn)
	}
	h.Sessions.SetTurn(seamTask, Turn{Thread: "t12", Depth: 2})
	h.Sessions.SetTurn(seamGroup, Turn{Jump: true})
	h.Sessions.SetTurn(seamTask, Turn{N: 40, Thread: "t12", Handback: true})
	if c, _ := h.Sessions.Caller(token); c.Turn != (Turn{N: 3, Thread: "t12", Handback: true}) {
		t.Fatalf("got %+v, want the third turn of this run, a hand-back in the thread of t12", c.Turn)
	}
	if c, _ := h.Sessions.Caller(other); c.Turn != (Turn{N: 2, Jump: true}) {
		t.Fatalf("got %+v, want the second turn of this run", c.Turn)
	}
	// A session made later for the same key starts without a turn.
	if err := h.Sessions.End(seamTask); err != nil {
		t.Fatal(err)
	}
	seamIn(t, h.Sessions, seamTask, Dormant)
	if c, ok := h.Sessions.Caller(h.Sessions.Admit(seamTask, 2, false)); !ok || c.Turn != (Turn{}) {
		t.Fatalf("got %+v, want no turn for a new session", c.Turn)
	}
}

func TestOpenCallsAreCountedPerSession(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	if h.Sessions.Calls(seamTask) != 0 {
		t.Fatal("a call is open before any began")
	}
	first, second, other := h.Sessions.Calling(seamTask), h.Sessions.Calling(seamTask), h.Sessions.Calling(seamGroup)
	if got := h.Sessions.Calls(seamTask); got != 2 {
		t.Fatalf("%d calls are open, want 2", got)
	}
	first()
	if got := h.Sessions.Calls(seamTask); got != 1 {
		t.Fatalf("%d calls are open, want 1", got)
	}
	second()
	other()
	if h.Sessions.Calls(seamTask) != 0 || h.Sessions.Calls(seamGroup) != 0 {
		t.Fatal("a call is open after all ended")
	}
}

// A takeover's audit row and its change of state commit together, or neither
// does (contracts/storage.md section 3).
func TestSessionChangesCommitWithTheirTransaction(t *testing.T) {
	db := seamStore(t)
	h, _ := seamHost(t, db)
	seamIn(t, h.Sessions, seamTask, Waiting)
	seamIn(t, h.Sessions, seamGroup, Dormant)
	token := h.Sessions.Admit(seamGroup, 1, false)
	h.Sessions.SetTurn(seamGroup, Turn{Thread: "t12"})
	left := h.Sessions.All()
	takeover := AuditRow{Group: "g1", Agent: "claude", What: AuditTakeover, Detail: "t12", By: "owner-device"}

	err := h.Sessions.Write(func(tx *Tx) error {
		if written, err := h.Audit(tx.Tx, takeover); err != nil || !written {
			t.Errorf("got %v and %v", written, err)
		}
		// Each change is in the table as soon as it is made, for whoever
		// reads the table inside the transaction.
		if err := tx.Create(seamSession(seamReview)); err != nil {
			t.Error(err)
		}
		if err := tx.Update(seamTask, func(s *Session) { s.TakeoverAt = "9f2c1ab" }); err != nil {
			t.Error(err)
		}
		if err := tx.End(seamGroup); err != nil {
			t.Error(err)
		}
		if _, ok := h.Sessions.Caller(token); ok || len(h.Sessions.All()) != 2 {
			t.Error("an ended session is still in the table inside the transaction")
		}
		// The session is waiting, so this takeover is refused, and with it
		// all of the transaction.
		return tx.Move(seamTask, Dormant, Takeover)
	})
	seamCode(t, err, "conflict")
	if got := h.Sessions.All(); !reflect.DeepEqual(got, left) {
		t.Fatalf("after the refused transaction the table holds %+v, want %+v", got, left)
	}
	if c, ok := h.Sessions.Caller(token); !ok || c.Turn != (Turn{N: 1, Thread: "t12"}) {
		t.Fatalf("got %+v, want the token and the turn the refused end took away", c)
	}
	if record := seamRecord(t, h); record != "" {
		t.Fatalf("the audit row of the refused takeover was kept: %q", record)
	}
	if _, saved := seamHost(t, db); !reflect.DeepEqual(saved, left) {
		t.Fatalf("stored %+v, want %+v", saved, left)
	}

	h, _ = seamHost(t, db)
	err = h.Sessions.Write(func(tx *Tx) error {
		if _, err := h.Audit(tx.Tx, takeover); err != nil {
			return err
		}
		return tx.Move(seamTask, Dormant, Takeover)
	})
	if err != nil || seamState(t, h.Sessions, seamTask) != Takeover {
		t.Fatalf("got %v and %s", err, seamState(t, h.Sessions, seamTask))
	}
	if record := seamRecord(t, h); record != "1763600000000|g1|claude|takeover|-|-|t12|owner-device|-" {
		t.Fatalf("got the audit record %q", record)
	}
	if _, saved := seamHost(t, db); saved[1].State != Takeover {
		t.Fatalf("stored %+v, want the task session in takeover", saved[1])
	}
}

// One lock order: no change waits for the store's writer while it holds the
// table's lock, so the table can be read inside any transaction of the store.
func TestSessionTableIsReadInsideATransaction(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seamIn(t, h.Sessions, seamTask, Dormant)
	token := h.Sessions.Admit(seamTask, 2, false)
	finished := make(chan error, 2)
	go func() {
		finished <- h.Store.Write(func(*store.Tx) error {
			// A change made meanwhile waits for this transaction to end.
			go func() { finished <- h.Sessions.Move(seamTask, Dormant, Working) }()
			time.Sleep(50 * time.Millisecond)
			if _, ok := h.Sessions.Get(seamTask); !ok || len(h.Sessions.All()) != 1 {
				t.Error("the table could not be read")
			}
			if c, ok := h.Sessions.Caller(token); !ok || c.State != Dormant {
				t.Errorf("got %+v, want the session before the change that waits", c)
			}
			return nil
		})
	}()
	for range 2 {
		select {
		case err := <-finished:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("a reader of the table and a change of it wait for each other")
		}
	}
	if seamState(t, h.Sessions, seamTask) != Working {
		t.Fatal("the change that waited was not made")
	}
}
