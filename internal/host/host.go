// Package host runs the AI sessions of a node: the CLI drivers, the session
// table, the MCP tools, the workspaces and the audit record
// (planning/architecture.md section 8). It imports neither peer nor node:
// node hands it connections behind the interfaces of this file, and calls
// what its tables hold. README.md says how a unit builds on it.
package host

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// A Link is what the host uses of a ctl connection. *peer.Connection has
// these methods, and so has the local link to a home on the same node.
type Link interface {
	// Send writes a push, or the answer to a request.
	Send(frame any) error
	// Path is "direct" or "relay" (flows/connect.md step 8), "local" on the
	// local link, and "" while no path is selected. The host reports the path
	// to a home in member status (contracts/commands.md 1.2), and refuses a
	// takeover unless the owner's is direct.
	Path() string
	// PathChanged is signalled whenever Path may have changed.
	PathChanged() <-chan struct{}
	// Done is closed when the connection has ended.
	Done() <-chan struct{}
}

// An Owner is a connection of a device of the node's owner: the peer of the
// host surface (contracts/datachannel.md section 7).
type Owner interface{ Link }

// A Home is the connection to a group home. The host is a client of the home
// whether the home is on another node or on this one.
type Home interface {
	Link
	// Request sends the request that build makes for the id it is given, and
	// returns the answer, a success or an error (contracts/conventions.md
	// section 3).
	Request(ctx context.Context, build func(id int64) any) (json.RawMessage, error)
}

// A Terminal is the byte stream of a pty channel (contracts/datachannel.md
// section 7). *peer.Stream has these methods.
type Terminal interface {
	io.ReadWriteCloser
	// Label is the channel's label, "pty:<g>:<member>".
	Label() string
	// Device is the device the connection was authenticated for.
	Device() string
}

// An Error is a failure with one of the codes of contracts/datachannel.md
// section 9. Its text, "<code>: <sentence>", is what a tool answers
// (contracts/mcp-tools.md).
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// A Host is the host of one node. Several can run in one process, so a unit
// keeps what it must remember in a Part, never in a package variable.
type Host struct {
	Store    *store.Store
	Key      identity.Key
	Sessions *Sessions
	Rows     *Rows
	Now      func() time.Time
	// Connect has node keep a connection to the home with that device id
	// open, again after every loss, until ctx ends; it returns at once. node
	// sets it, and calls Attach for each connection that opens. For a home on
	// this node the connection is the local link.
	Connect func(ctx context.Context, device string)
	// WatchEnded notifies node when a person's last watch grant ends.
	WatchEnded func(group, user string)
	// Shim is the command line that starts this node's MCP shim, without the
	// token (contracts/local.md section 1). node sets it, so that the CLIs of
	// two nodes in one process reach each their own node.
	Shim []string

	mu      sync.Mutex
	homes   map[string]Home
	waiting sync.Mutex
	waits   map[string]*wait
	parts   sync.Map
}

// New loads the session table as the node starts (sessions.md section 2).
// saved holds the sessions as the last run left them: the caller reports a
// crash for each that was working, and hands back each that was in takeover.
func New(db *store.Store, key identity.Key, now func() time.Time) (h *Host, saved []Session, err error) {
	sessions, saved, err := loadSessions(db, now)
	if err != nil {
		return nil, nil, err
	}
	h = &Host{Store: db, Key: key, Sessions: sessions, Rows: &Rows{rows: map[rowKey]json.RawMessage{}}, Now: now,
		homes: map[string]Home{}, waits: map[string]*wait{}}
	for _, built := range inOrder(Built) {
		built(h)
	}
	return h, saved, nil
}

// Built holds what is to happen once when a host is built, such as loading
// what a unit keeps in memory from the store. Each entry has the name of its
// unit; New calls them in the order of their names.
var Built = map[string]func(h *Host){}

// Part returns what one unit keeps for this host: one value per type, made
// from its zero value at the first call.
func Part[T any](h *Host) *T {
	part, _ := h.parts.LoadOrStore(reflect.TypeFor[T](), new(T))
	return part.(*T)
}

// Register adds an entry to one of the tables of this file, from the init of
// the file that provides it. A key has one entry: a second one panics, so
// that two units cannot both claim a tool, a frame or a command.
func Register[V any](table map[string]V, key string, entry V) {
	if _, taken := table[key]; taken {
		panic("host: " + key + " is registered twice")
	}
	table[key] = entry
}

// Provide sets one of the hooks of this file, from the init of the file that
// implements it. A hook has one provider: a second one panics.
func Provide[F any](hook *F, provided F) {
	if !reflect.ValueOf(hook).Elem().IsNil() {
		panic("host: a hook is provided twice")
	}
	*hook = provided
}

// inOrder returns the entries of a table in the order of their keys.
func inOrder[V any](table map[string]V) []V {
	entries := make([]V, 0, len(table))
	for _, key := range slices.Sorted(maps.Keys(table)) {
		entries = append(entries, table[key])
	}
	return entries
}

// Attached holds what is to happen each time a connection to a home opens,
// such as reading the state of the groups homed there. Each entry has the
// name of its unit; Attach calls them in the order of their names. home is
// the device id of the home.
var Attached = map[string]func(h *Host, home string){}

// Attach makes home the connection to the home with that device id, in place
// of an earlier one. node calls it for each connection it opens to a home, and
// with the local link for a home on this node.
func (h *Host) Attach(device string, home Home) {
	h.mu.Lock()
	h.homes[device] = home
	h.mu.Unlock()
	for _, attached := range inOrder(Attached) {
		attached(h, device)
	}
}

// Settled holds what is to wait for background work of this host before its
// store closes. Each entry has the name of its unit; Settle calls them in the
// order of their names.
var Settled = map[string]func(h *Host){}

// Settle waits for background work of this host to finish before its store
// closes.
func (h *Host) Settle() {
	for _, settle := range inOrder(Settled) {
		settle(h)
	}
}

// Home returns the connection to the home with that device id while it is
// open.
func (h *Host) Home(device string) (Home, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	home, ok := h.homes[device]
	if !ok {
		return nil, false
	}
	select {
	case <-home.Done():
		return nil, false
	default:
		return home, true
	}
}

// ctlLimit is the size of a ctl message at most (contracts/conventions.md
// section 4).
const ctlLimit = 61_440

var errHomeUnreachable = &Error{Code: "home_unreachable", Message: "the home of that group is not connected"}

// Command signs payload, the JSON text of a command, with the node's key and
// sends it to the home with that device id (contracts/datachannel.md section
// 5). It returns the home's answer when the command was accepted, the home's
// error as an *Error when it was refused, and home_unreachable when no answer
// came (flows/agent-turn.md F). A command too large for a ctl message is not
// sent: it fails with bad_request, which names the limit and the size.
func (h *Host) Command(ctx context.Context, device, payload string) (json.RawMessage, error) {
	home, ok := h.Home(device)
	if !ok {
		return nil, errHomeUnreachable
	}
	env := h.Key.Sign(payload)
	frame := func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	}
	if size := frameSize(frame(math.MaxInt64)); size > ctlLimit {
		return nil, &Error{"bad_request", fmt.Sprintf("the command takes %d bytes as a ctl message, and the limit is %d", size, ctlLimit)}
	}
	answer, err := home.Request(ctx, frame)
	if err != nil {
		return nil, errHomeUnreachable
	}
	var refused proto.ErrorResponse
	if proto.Decode(answer, &refused) == nil {
		return nil, &Error{refused.Err.Code, refused.Err.Message}
	}
	return answer, nil
}

// frameSize is the size of a frame as a connection encodes it: JSON without
// HTML escaping (contracts/conventions.md section 4). The frames of this
// package always encode.
func frameSize(frame any) int {
	var text bytes.Buffer
	encoder := json.NewEncoder(&text)
	encoder.SetEscapeHTML(false)
	encoder.Encode(frame)
	return text.Len() - 1 // without the line end the encoder adds
}

// A wait is one tool call that waits for the item that answers a message.
type wait struct {
	check func(item json.RawMessage) error
	item  chan json.RawMessage // buffered for 1 item; single reader is Await's caller
}

// Await opens the wait of a tool call for the item that answers the message
// with that id: the reply to a call whose ask is open, the decision of an
// approval the CLI is waiting on. Such an item is not a turn
// (flows/agent-turn.md A). The item arrives on the channel, one at most.
// check, when it is not nil, sees the item first and refuses it by its
// error: a decision whose cause no approver signed. done ends the wait; an
// item handed over before it is still in the channel. An id has one wait: a
// second Await for it takes the place of the first.
func (h *Host) Await(id string, check func(item json.RawMessage) error) (item <-chan json.RawMessage, done func()) {
	w := &wait{check, make(chan json.RawMessage, 1)}
	h.waiting.Lock()
	defer h.waiting.Unlock()
	h.waits[id] = w
	return w.item, func() { h.release(id, w) }
}

// release ends a wait when it is still open.
func (h *Host) release(id string, w *wait) {
	h.waiting.Lock()
	defer h.waiting.Unlock()
	if h.waits[id] == w {
		delete(h.waits, id)
	}
}

// Hand gives an item to the tool call that waits for the answer to the
// message with that id, and ends that wait. Delivery calls it before it
// queues a reply or a decided item, with the item as the home sent it.
// waited is false when nothing waits, or the wait ended while the item was
// checked: the item then takes its turn in the queue. Otherwise err says
// whether the item was handed over or refused by the wait's check. Hand
// holds no lock while the check runs, and never waits for the receiver.
func (h *Host) Hand(id string, item json.RawMessage) (waited bool, err error) {
	h.waiting.Lock()
	w := h.waits[id]
	h.waiting.Unlock()
	if w == nil {
		return false, nil
	}
	if w.check != nil {
		if err := w.check(item); err != nil {
			return true, err
		}
	}
	h.waiting.Lock()
	defer h.waiting.Unlock()
	if h.waits[id] != w {
		return false, nil
	}
	delete(h.waits, id)
	w.item <- item
	return true, nil
}

// Holds says whether a session's deliveries are held, by the name of the
// unit that knows of one kind of hold: the unit of limits for an AI that is
// rate limited, or offline by the owner's command (flows/agent-turn.md step
// 3). The session table itself holds an AI that is
// working or taken over (Sessions.Move).
var Holds = map[string]func(h *Host, key SessionKey) (reason string, held bool){}

// Held asks every entry of Holds, in the order of their names, and returns
// the first hold. Delivery calls it before it gives a session a turn;
// whoever lifts a hold calls Deliver.
func (h *Host) Held(key SessionKey) (reason string, held bool) {
	for _, hold := range inOrder(Holds) {
		if reason, held := hold(h, key); held {
			return reason, true
		}
	}
	return "", false
}

type rowKey struct{ group, kind, id string }

// Rows is the host's copy of the state rows of the groups its AIs are in:
// the group, member, device and task rows a home sends for a sync with
// state_only (contracts/datachannel.md section 4). The unit that handles
// rows and reset keeps it, by the rules of that section; every unit reads
// it. A row is the JSON text the home sent.
type Rows struct {
	mu   sync.Mutex
	rows map[rowKey]json.RawMessage
}

// Put stores a row of a group under its type r and its id, in place of the
// one held.
func (c *Rows) Put(group, r, id string, row json.RawMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows[rowKey{group, r, id}] = row
}

// Reset drops every row of a group.
func (c *Rows) Reset(group string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	maps.DeleteFunc(c.rows, func(key rowKey, _ json.RawMessage) bool { return key.group == group })
}

// Get returns one row of a group.
func (c *Rows) Get(group, r, id string) (json.RawMessage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	row, ok := c.rows[rowKey{group, r, id}]
	return row, ok
}

// Of returns the rows of one type of a group, in the order of their ids.
func (c *Rows) Of(group, r string) []json.RawMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	byID := map[string]json.RawMessage{}
	for key, row := range c.rows {
		if key.group == group && key.kind == r {
			byID[key.id] = row
		}
	}
	return inOrder(byID)
}

// Pushes holds the handler of each kind of frame a home pushes to a host, by
// the frame's t: "inbox", "rows", "reset". home is the device id of the home.
var Pushes = map[string]func(h *Host, home string, frame json.RawMessage){}

// Push hands a frame that a home pushed to the handler of its kind. node
// calls it for each push it reads on a connection to a home, and the local
// link for each push of a home on this node. A frame of a kind nobody handles
// is dropped (contracts/datachannel.md section 2).
func (h *Host) Push(home string, frame json.RawMessage) {
	var kind struct {
		Type string `json:"t"`
	}
	if proto.Decode(frame, &kind) != nil {
		return
	}
	if handle := Pushes[kind.Type]; handle != nil {
		handle(h, home, frame)
	}
}

// Surface holds what the host answers on the host surface
// (contracts/datachannel.md section 7), by t: the t of a request or a push
// ("audit", "pty.resize"), or the t of a host command's payload
// ("takeover.start", contracts/commands.md section 2).
//
// node calls a handler only for a device of the owner, by: for a command,
// after the envelope checks and the receipt lookup, with the payload as data;
// otherwise with the frame. The result holds the result fields of the answer,
// nil when there are none; an *Error is answered with its code, any other
// error with internal.
var Surface = map[string]func(ctx context.Context, h *Host, from Owner, by string, data json.RawMessage) (any, error){}

// A Tool is one MCP tool of the server named solodawn
// (contracts/mcp-tools.md).
type Tool struct {
	Description string
	Schema      json.RawMessage // the JSON Schema of its arguments
	// For says whether the tool is listed to a caller; nil lists it to all.
	For func(c Caller) bool
	// Call runs the tool. Its result is encoded as the JSON of the one text
	// block; an *Error is answered as its text, any other error as internal.
	Call func(ctx context.Context, h *Host, c Caller, args json.RawMessage) (any, error)
}

// Tools holds every tool by name.
var Tools = map[string]Tool{}

// ToolsFor returns the tools the server lists to a caller. A tool it does not
// list to a caller is not called for that caller either. The server brackets
// each call with Sessions.Calling.
func ToolsFor(c Caller) map[string]Tool {
	listed := maps.Clone(Tools)
	maps.DeleteFunc(listed, func(_ string, tool Tool) bool { return tool.For != nil && !tool.For(c) })
	return listed
}

// Drivers holds the driver of each CLI by its agent name; see DriverCaps.
var Drivers = map[string]Driver{}

// A Driver starts one vendor's CLI (contracts/drivers.md section 1).
type Driver interface {
	// Start starts the structured CLI of a session. The process lives no
	// longer than ctx: when ctx ends, its process tree is killed.
	Start(ctx context.Context, l Launch) (Process, error)
	// Interactive returns the command line that opens the session in a
	// terminal, for a takeover (contracts/drivers.md 1.3), and the session
	// id it names, which the caller stores: a terminal reports nothing. It
	// fails where the CLI cannot open in a terminal a session that has had
	// no turn.
	Interactive(l Launch) (argv []string, session string, err error)
}

// A Launch is what a driver is told about the session it starts.
type Launch struct {
	Folder       string        // the session's working folder, an absolute path
	Session      string        // the CLI's session or thread id; "" before the first start
	Level        int           // 1, 2 or 3 (contracts/drivers.md section 1)
	Shim         []string      // the command line of the MCP server of this start: Host.Shim, then --token and the session token
	Instructions string        // the text of the session kind (contracts/drivers.md section 4)
	RepoConfig   bool          // load the repository's own AI configuration: task and review sessions only (R-45)
	ToolTimeout  time.Duration // how long a call of one of the node's tools may last
}

// A Process is one running structured CLI. Its caller owns the timing: it
// kills, by ending the context of Start, a process that has not answered an
// interrupt or a close within 10 s (flows/agent-turn.md G).
type Process interface {
	// Events is closed when the process has exited and all it wrote is read.
	Events() <-chan Event
	// Turn writes one turn (contracts/drivers.md section 3).
	Turn(text string) error
	// Interrupt asks the CLI to end the turn; a TurnEnded with the status
	// "interrupted" follows.
	Interrupt() error
	// Answer allows or declines the permission request with that id.
	Answer(ask string, allow bool) error
	// Close closes the CLI's input, after which it exits by itself.
	Close() error
}

// An EventKind names what happened in the CLI process of a session.
type EventKind string

const (
	Ready        EventKind = "ready"         // the CLI has a session: Session is its id, to store for the next start
	Active       EventKind = "active"        // the CLI sent something; a silent turn is one without it (contracts/drivers.md 1.2)
	Asked        EventKind = "asked"         // a permission request: Ask, to be answered with Process.Answer
	Used         EventKind = "used"          // usage figures outside a turn's end: Usage
	TurnBegan    EventKind = "turn_began"    // a turn was written; reported by the runtime, not by a driver
	TurnEnded    EventKind = "turn_ended"    // the turn ended: Status and Usage
	RateLimited  EventKind = "rate_limited"  // the vendor's rate limit was reached: Resets
	LoginInvalid EventKind = "login_invalid" // sign-in is not usable in this mode (R-56): Text
	NoSession    EventKind = "no_session"    // the CLI cannot resume the session id: Text
)

// An Event is one report about the CLI process of a session. Only the fields
// its kind names are set.
type Event struct {
	Kind    EventKind
	Session string // Ready
	Status  string // TurnEnded: "completed", "interrupted" or "failed"
	Ask     *Ask   // Asked
	Usage   Usage  // Used, TurnEnded
	Resets  int64  // RateLimited: when the window resets, in Unix milliseconds
	Text    string // the CLI's own error text
}

// Usage holds the figures a CLI reported; one it did not report is nil.
type Usage struct {
	Tokens       *int64   // tokens of the turn
	USD          *float64 // cost of the turn
	WindowPct    *float64 // share of the vendor's rate-limit window that is used
	WindowResets *int64   // when that window resets, in Unix milliseconds
}

// An Ask is a permission request of a CLI in the one form both CLIs' requests
// take (flows/approval.md A). Tool, Category, Command and Why are the fields
// of the approval card (contracts/messages.md 2.3), not yet shortened.
type Ask struct {
	ID       string          // names the request in Runtime.Answer; new on every attempt
	Tool     string          // what the CLI names
	Category string          // command, file, network or other
	Command  string          // the request as the CLI presented it
	Why      string          // the justification the CLI attached; "" without one
	Input    json.RawMessage // what the digest of the approval record covers (flows/approval.md step 5)
	// Reply answers a CLI user-input question by question id, or nil when unanswered.
	Reply func(map[string][]string) error
}

// Listeners are told every event of every session's CLI. Each has the name
// of its unit; Report calls them in the order of their names.
var Listeners = map[string]func(h *Host, key SessionKey, e Event){}

// Report tells the listeners an event of the CLI of a session. The runtime
// calls it for each event it reads from Process.Events, in their order, and
// for TurnBegan. A listener returns at once; what takes time it starts on a
// goroutine of its own.
func (h *Host) Report(key SessionKey, e Event) {
	for _, listen := range inOrder(Listeners) {
		listen(h, key, e)
	}
}

// The Runtime owns the CLI processes of the sessions. The session runtime
// provides Runner; delivery, the approval handler and the takeover call it.
type Runtime interface {
	// Turn gives a session one turn, with the text and the turn that Compose
	// made. It starts the session's CLI when the session is dormant, as a
	// resume when the CLI has a session id; notes turn with
	// Sessions.SetTurn; writes text; and reports TurnBegan. It returns once
	// the turn is written. With turn.Jump it first interrupts the turn the
	// AI is running in the group (flows/agent-turn.md C). When the turn has
	// ended, the runtime reports TurnEnded and calls Deliver.
	Turn(ctx context.Context, h *Host, key SessionKey, text string, turn Turn) error
	// Answer allows or declines a permission request that the CLI of a
	// session made: the event Asked.
	Answer(h *Host, key SessionKey, ask string, allow bool) error
	// Close closes any open CLI process for key and waits for it to exit
	// (flows/agent-turn.md C-6, D17, G).
	Close(h *Host, key SessionKey)
}

// The hooks through which the units of the session runtime call each other.
// Each is set with Provide. A test of a unit that calls one sets a fake.
var (
	// Runner is the session runtime.
	Runner Runtime
	// Deliver gives an AI in a group its next turn, when items are held for
	// it and nothing holds them any more (flows/agent-turn.md step 10).
	// Delivery provides it. The runtime calls it when a turn of the AI has
	// ended, and whoever lifts a hold calls it too.
	Deliver func(h *Host, group, member string)
	// Instructions returns the instructions a session is started with, by
	// its kind (contracts/drivers.md section 4).
	Instructions func(s Session) string
	// Compose makes one turn of a session: its text, with the header every
	// turn has (contracts/drivers.md section 3), and what the host notes
	// about it. For a turn made of inbox items it is given them in their
	// order, each one element of the items of an inbox frame as the home
	// sent it (contracts/datachannel.md section 6); the other kinds have
	// none.
	Compose func(h *Host, s Session, kind TurnKind, items []json.RawMessage) (text string, turn Turn, err error)
)
