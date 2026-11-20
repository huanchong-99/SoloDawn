package host

import (
	"cmp"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"slices"
	"sync"
	"time"

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
	CLISession string `json:"cli_session"` // "" until the CLI's first start reports it
	State      State  `json:"state"`
	LastActive int64  `json:"last_active"` // the last change of state, in Unix milliseconds
	TakeoverAt string `json:"takeover_at"` // the commit noted when a takeover started; "" otherwise
}

// A Caller is the CLI process that a session token names: the session as it
// is now, and how that process was started.
type Caller struct {
	Session
	Level       int  // the level the process was started with
	Interactive bool // started in a terminal, for a takeover
}

type admission struct {
	key         SessionKey
	level       int
	interactive bool
}

// Sessions is the session table of a host, kept in memory and written through
// to the store. Its methods may be called from any goroutine.
type Sessions struct {
	db     *store.Store
	now    func() time.Time
	mu     sync.Mutex
	rows   map[SessionKey]Session
	tokens map[string]admission
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
	t = &Sessions{db: db, now: now, rows: map[SessionKey]Session{}, tokens: map[string]admission{}}
	for _, s := range saved {
		if s.State != Takeover {
			s.State = Dormant
		}
		t.rows[s.SessionKey] = s
	}
	return t, saved, err
}

// put writes a row to the store, and then to memory.
func (t *Sessions) put(s Session) error {
	err := t.db.Write(func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT OR REPLACE INTO session(g, member, task, kind, folder, agent, home, name, secretary,
			cli_session, state, last_active, takeover_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, nullif(?, ''), ?, ?, nullif(?, ''))`,
			s.Group, s.Member, s.Task, s.Kind, s.Folder, s.Agent, s.Home, s.Name, s.Secretary,
			s.CLISession, s.State, s.LastActive, s.TakeoverAt)
		return err
	})
	if err == nil {
		t.rows[s.SessionKey] = s
	}
	return err
}

// change applies edit to the row of key and writes the result, unless there
// is no such session or edit refuses.
func (t *Sessions) change(key SessionKey, edit func(*Session) error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.rows[key]
	if !ok {
		return &Error{"not_found", "no such session"}
	}
	if err := edit(&s); err != nil {
		return err
	}
	return t.put(s)
}

// Create records a new session, dormant. A second session for one key is
// refused with conflict.
func (t *Sessions) Create(s Session) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.rows[s.SessionKey]; ok {
		return &Error{"conflict", "the session exists already"}
	}
	s.State, s.LastActive = Dormant, t.now().UnixMilli()
	return t.put(s)
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

func byKey(a, b Session) int {
	return cmp.Or(cmp.Compare(a.Group, b.Group), cmp.Compare(a.Member, b.Member), cmp.Compare(a.Task, b.Task))
}

// Update changes what a session records beside its key and its state, which
// only Move changes: the CLI's session id, the commit of a takeover.
func (t *Sessions) Update(key SessionKey, edit func(*Session)) error {
	return t.change(key, func(s *Session) error {
		state, at := s.State, s.LastActive
		edit(s)
		s.SessionKey, s.State, s.LastActive = key, state, at
		return nil
	})
}

// Move changes the state of a session from one state to another. It refuses
// with conflict a session that is not in the state from, and a change that
// sessions.md section 2 does not list. It refuses with busy a change to
// working or takeover while another session of the same AI in the same group
// is in either: an AI runs one turn at a time per group (sessions.md section
// 1), and a takeover holds its other sessions (flows/takeover.md step 9).
func (t *Sessions) Move(key SessionKey, from, to State) error {
	return t.change(key, func(s *Session) error {
		if s.State != from || !slices.Contains(changes[from], to) {
			return &Error{"conflict", "the session is " + string(s.State) + ": no change from " + string(from) + " to " + string(to)}
		}
		if active(to) && t.engaged(key) {
			return &Error{"busy", "another session of this AI in the group is working or taken over"}
		}
		s.State, s.LastActive = to, t.now().UnixMilli()
		return nil
	})
}

// active reports whether a process is doing work in that state: a turn, or
// the owner's terminal.
func active(s State) bool { return s == Working || s == Takeover }

// engaged reports whether another session of the AI of key, in its group, is
// active.
func (t *Sessions) engaged(key SessionKey) bool {
	for other, s := range t.rows {
		if other != key && other.Group == key.Group && other.Member == key.Member && active(s.State) {
			return true
		}
	}
	return false
}

// End removes a session that has ended (sessions.md section 1). Its tokens
// name nobody from then on.
func (t *Sessions) End(key SessionKey) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	err := t.db.Write(func(tx *store.Tx) error {
		_, err := tx.Exec("DELETE FROM session WHERE g = ? AND member = ? AND task = ?", key.Group, key.Member, key.Task)
		return err
	})
	if err == nil {
		delete(t.rows, key)
	}
	return err
}

// Admit issues the session token of a CLI process that is about to start for
// a session, at that level, structured or in a terminal: 16 random bytes,
// kept in memory only (contracts/local.md section 3). Whoever starts the
// process calls Forget when it has exited.
func (t *Sessions) Admit(key SessionKey, level int, interactive bool) string {
	var random [16]byte
	rand.Read(random[:])
	token := base64.RawURLEncoding.EncodeToString(random[:])
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tokens[token] = admission{key, level, interactive}
	return token
}

// Forget drops a token.
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
	a, issued := t.tokens[token]
	s, exists := t.rows[a.key]
	return Caller{Session: s, Level: a.level, Interactive: a.interactive}, issued && exists
}
