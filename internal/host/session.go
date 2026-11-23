package host

import (
	"cmp"
	"encoding/json"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// A Kind is one of the three kinds of session (sessions.md section 1).
type Kind string

const (
	GroupSession  Kind = "group"
	TaskSession   Kind = "task"
	ReviewSession Kind = "review"
)

// A State is one of the four states of a session (sessions.md section 2).
type State string

const (
	Dormant  State = "dormant"
	Working  State = "working"
	Waiting  State = "waiting"
	Takeover State = "takeover"
)

// changes lists every change of state of sessions.md section 2, by the state
// it starts from. The changes "when the node starts" are made by loadSessions
// (to dormant) and by the hand-back that follows it (takeover to working).
var changes = map[State][]State{
	Dormant:  {Working, Takeover},
	Waiting:  {Working, Dormant, Takeover},
	Working:  {Waiting, Dormant, Takeover},
	Takeover: {Working, Dormant},
}

// A SessionKey names one session of an AI. Task is "" for the group session.
type SessionKey struct {
	Group  string `json:"g"`
	Member string `json:"member"`
	Task   string `json:"task"`
}

// A Session is one row of the session table (contracts/storage.md section 3).
type Session struct {
	SessionKey
	Kind       Kind   `json:"kind"`
	Folder     string `json:"folder"`      // relative to projects/<group>/
	Agent      string `json:"agent"`       // the key of its driver in Drivers
	Home       string `json:"home"`        // device id of the group home
	Name       string `json:"name"`        // the AI's display name
	Secretary  bool   `json:"secretary"`   // the AI is the group's secretary
	CLISession string `json:"cli_session"` // "" until the CLI's first start
	State      State  `json:"state"`
	LastActive int64  `json:"last_active"` // the last change of state, in Unix milliseconds
	TakeoverAt string `json:"takeover_at"` // the commit noted when a takeover started; "" otherwise
}

// A TurnKind says what a turn is made of. Every kind has the same header
// (contracts/drivers.md section 3).
type TurnKind string

const (
	ItemsTurn    TurnKind = "items"    // inbox items (flows/agent-turn.md step 6)
	ReplacedTurn TurnKind = "replaced" // the first turn of a session that replaces one the CLI could not resume (flows/agent-turn.md E)
	ContinueTurn TurnKind = "continue" // the turn of its own a session gets after a jump interrupted it (flows/agent-turn.md step 14)
	HandbackTurn TurnKind = "handback" // the turn that follows a takeover (flows/takeover.md step 16)
)

// A Turn is what the host notes about a turn beside its text. The tools
// called in that turn go by it (Caller.Turn).
type Turn struct {
	N      int64    // the number of the turn among all turns of this host's run; set by SetTurn
	Kind   TurnKind // "" before the session's first turn
	Thread string   // the task whose thread the turn's last item is in; "" for the main line
	Depth  int      // the depth of the call the turn answers; 0 when it answers none (flows/ask.md step 2)
	Jump   bool     // the turn jumps the queue (flows/agent-turn.md C)
}

// A Caller is the CLI process that a session token names: the session as it
// is now, how that process was started, and the turn the session was given
// last.
type Caller struct {
	Session
	Level       int  // the level the process was started with
	Interactive bool // started in a terminal, for a takeover
	External    bool // an external AI, by its member id
	Turn        Turn
}

// An admission is what a token stands for. ended is set when its session
// has ended: the token then names nobody, also not a session made later for
// the same key.
type admission struct {
	key         SessionKey
	level       int
	interactive bool
	ended       bool
}

// Sessions is the session table of a host, kept in memory and written through
// to the store. Its methods may be called from any goroutine.
//
// Lock order: a change runs on the store's one writer and takes the table's
// lock only there, inside the transaction; nothing waits for the writer while
// it holds the lock. So Get, All and Caller may be called anywhere, inside a
// transaction of the store too.
type Sessions struct {
	db     *store.Store
	now    func() time.Time
	mu     sync.Mutex
	rows   map[SessionKey]Session
	tokens map[string]*admission
	turns  map[SessionKey]Turn
	turned int64
	calls  map[SessionKey]int
}

const readSessions = `SELECT json_group_array(json_object('g', g, 'member', member, 'task', task, 'kind', kind,
	'folder', folder, 'agent', agent, 'home', home, 'name', name, 'secretary', json(iif(secretary, 'true', 'false')),
	'cli_session', coalesce(cli_session, ''), 'state', state, 'last_active', last_active,
	'takeover_at', coalesce(takeover_at, ''))) FROM session`

// loadSessions reads the table as the node starts. No process survived the
// last run, so a session saved as working or waiting becomes dormant. One
// saved as takeover keeps that state, which holds its deliveries, until its
// hand-back (flows/takeover.md E). saved holds the rows as they were, in the
// order of All.
func loadSessions(db *store.Store, now func() time.Time) (t *Sessions, saved []Session, err error) {
	var text string
	err = db.Write(func(tx *store.Tx) error {
		if err := tx.QueryRow(readSessions).Scan(&text); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE session SET state = ? WHERE state IN (?, ?)", Dormant, Working, Waiting)
		return err
	})
	if err == nil {
		err = json.Unmarshal([]byte(text), &saved)
	}
	slices.SortFunc(saved, byKey)
	t = &Sessions{db: db, now: now, rows: map[SessionKey]Session{}, tokens: map[string]*admission{},
		turns: map[SessionKey]Turn{}, calls: map[SessionKey]int{}}
	for _, s := range saved {
		if s.State != Takeover {
			s.State = Dormant
		}
		t.rows[s.SessionKey] = s
	}
	return t, saved, err
}

func byKey(a, b Session) int {
	return cmp.Or(cmp.Compare(a.Group, b.Group), cmp.Compare(a.Member, b.Member), cmp.Compare(a.Task, b.Task))
}

// A Tx is one transaction of the store in which sessions change together with
// other rows: a takeover's audit row and the change of state it records
// commit together or not at all. Its embedded transaction is the store's.
type Tx struct {
	*store.Tx
	t    *Sessions
	undo []func() // each puts back what a change took from memory alone
}

// Write runs one transaction of the store, on the store's one writer. What
// run changes through tx is in the table at once, and is taken out of it
// again when run fails.
//
// Inside run, change sessions with tx. Create, Update, Move and End of
// Sessions each start a transaction of their own, and like every Write of
// the store, one started inside another waits for ever.
func (t *Sessions) Write(run func(tx *Tx) error) error {
	return t.db.Write(func(inner *store.Tx) error {
		tx := &Tx{Tx: inner, t: t}
		t.mu.Lock()
		before := maps.Clone(t.rows)
		t.mu.Unlock()
		err := run(tx)
		if err != nil {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.rows = before
			for _, undo := range tx.undo {
				undo()
			}
		}
		return err
	})
}

// set writes a row to the store, and then to memory. The caller holds the
// table's lock.
func (tx *Tx) set(s Session) error {
	_, err := tx.Exec(`INSERT OR REPLACE INTO session(g, member, task, kind, folder, agent, home, name, secretary,
		cli_session, state, last_active, takeover_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, nullif(?, ''), ?, ?, nullif(?, ''))`,
		s.Group, s.Member, s.Task, s.Kind, s.Folder, s.Agent, s.Home, s.Name, s.Secretary,
		s.CLISession, s.State, s.LastActive, s.TakeoverAt)
	if err == nil {
		tx.t.rows[s.SessionKey] = s
	}
	return err
}

// change applies edit to the row of key and writes the result, unless there
// is no such session or edit refuses.
func (tx *Tx) change(key SessionKey, edit func(*Session) error) error {
	tx.t.mu.Lock()
	defer tx.t.mu.Unlock()
	s, ok := tx.t.rows[key]
	if !ok {
		return &Error{"not_found", "no such session"}
	}
	if err := edit(&s); err != nil {
		return err
	}
	return tx.set(s)
}

// Create records a new session, dormant. A second session for one key is
// refused with conflict.
func (tx *Tx) Create(s Session) error {
	tx.t.mu.Lock()
	defer tx.t.mu.Unlock()
	if _, ok := tx.t.rows[s.SessionKey]; ok {
		return &Error{"conflict", "the session exists already"}
	}
	s.State, s.LastActive = Dormant, tx.t.now().UnixMilli()
	return tx.set(s)
}

// Update changes what a session records beside its state: the CLI's session
// id, the commit of a takeover. The state is Move's to change.
func (tx *Tx) Update(key SessionKey, edit func(*Session)) error {
	return tx.change(key, func(s *Session) error {
		edit(s)
		return nil
	})
}

// Move changes the state of a session from one state to another. It refuses
// with conflict a session that is not in the state from, and a change that
// sessions.md section 2 does not list. It refuses with busy a change to
// working or takeover while another session of the same AI in the same group
// is in either: an AI runs one turn at a time per group (sessions.md section
// 1), and a takeover holds its other sessions (flows/takeover.md step 9).
func (tx *Tx) Move(key SessionKey, from, to State) error {
	return tx.change(key, func(s *Session) error {
		if s.State != from || !slices.Contains(changes[from], to) {
			return &Error{"conflict", "the session is " + string(s.State) + ": no change from " + string(from) + " to " + string(to)}
		}
		if active(to) && tx.t.engaged(key) {
			return &Error{"busy", "another session of this AI in the group is working or taken over"}
		}
		s.State, s.LastActive = to, tx.t.now().UnixMilli()
		return nil
	})
}

// active reports whether a process is doing work in that state: a turn, or
// the owner's terminal.
func active(s State) bool { return s == Working || s == Takeover }

// engaged reports whether another session of the AI of key, in its group, is
// active. The caller holds the table's lock.
func (t *Sessions) engaged(key SessionKey) bool {
	for other, s := range t.rows {
		if other != key && other.Group == key.Group && other.Member == key.Member && active(s.State) {
			return true
		}
	}
	return false
}

// End removes a session that has ended (sessions.md section 1), with the
// turn noted for it, and ends the tokens issued for it: a session made later
// for the same key starts with neither.
func (tx *Tx) End(key SessionKey) error {
	t := tx.t
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := tx.Exec("DELETE FROM session WHERE g = ? AND member = ? AND task = ?", key.Group, key.Member, key.Task); err != nil {
		return err
	}
	turn, ended := t.turns[key], []*admission{}
	for _, a := range t.tokens {
		if a.key == key && !a.ended {
			a.ended = true
			ended = append(ended, a)
		}
	}
	delete(t.rows, key)
	delete(t.turns, key)
	// A token that was forgotten meanwhile is no longer in the table, so
	// that taking this back does not bring it back.
	tx.undo = append(tx.undo, func() {
		for _, a := range ended {
			a.ended = false
		}
		t.turns[key] = turn
	})
	return nil
}

// Create is Tx.Create in a transaction of its own.
func (t *Sessions) Create(s Session) error {
	return t.Write(func(tx *Tx) error { return tx.Create(s) })
}

// Update is Tx.Update in a transaction of its own.
func (t *Sessions) Update(key SessionKey, edit func(*Session)) error {
	return t.Write(func(tx *Tx) error { return tx.Update(key, edit) })
}

// Move is Tx.Move in a transaction of its own.
func (t *Sessions) Move(key SessionKey, from, to State) error {
	return t.Write(func(tx *Tx) error { return tx.Move(key, from, to) })
}

// End is Tx.End in a transaction of its own.
func (t *Sessions) End(key SessionKey) error {
	return t.Write(func(tx *Tx) error { return tx.End(key) })
}

// Get returns the session of key.
func (t *Sessions) Get(key SessionKey) (Session, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.rows[key]
	return s, ok
}

// All returns every session, ordered by group, member and task.
func (t *Sessions) All() []Session {
	t.mu.Lock()
	defer t.mu.Unlock()
	all := make([]Session, 0, len(t.rows))
	for _, s := range t.rows {
		all = append(all, s)
	}
	slices.SortFunc(all, byKey)
	return all
}

// Admit issues the session token of a CLI process that is about to start for
// a session, at that level, structured or in a terminal: 16 random bytes,
// kept in memory only (contracts/local.md section 3). Whoever starts the
// process calls Forget when it has exited.
func (t *Sessions) Admit(key SessionKey, level int, interactive bool) string {
	token := string(proto.NewNonce())
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tokens[token] = &admission{key: key, level: level, interactive: interactive}
	return token
}

// Forget drops a token, for good: an End that is taken back does not bring
// it back.
func (t *Sessions) Forget(token string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.tokens, token)
}

// Caller returns the process a token names, with its session as it is now. A
// token that was forgotten, or whose session has ended, names nobody.
func (t *Sessions) Caller(token string) (Caller, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	a := t.tokens[token]
	if a == nil || a.ended {
		return Caller{}, false
	}
	s, exists := t.rows[a.key]
	return Caller{Session: s, Level: a.level, Interactive: a.interactive, Turn: t.turns[a.key]}, exists
}

// SetTurn notes the turn a session is given and numbers it. Whoever writes
// the turn to the CLI calls it first.
func (t *Sessions) SetTurn(key SessionKey, turn Turn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.turned++
	turn.N = t.turned
	t.turns[key] = turn
}

// Calling marks that a call of one of the node's tools, or a permission
// request, is open for a session until done is called, once. While one is
// open a turn is not silent (contracts/drivers.md 1.2, step 6).
func (t *Sessions) Calling(key SessionKey) (done func()) {
	t.count(key, 1)
	return func() { t.count(key, -1) }
}

func (t *Sessions) count(key SessionKey, by int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls[key] += by
}

// Calls returns how many calls and requests are open for a session.
func (t *Sessions) Calls(key SessionKey) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls[key]
}
