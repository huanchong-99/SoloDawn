// Package host runs the AI sessions of a node: the CLI drivers, the session
// table, the MCP tools, the workspaces and the audit record
// (planning/architecture.md section 8). It imports neither peer nor node:
// node hands it connections behind the interfaces of this file, and calls
// what its tables hold. README.md says how a unit builds on it.
package host

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"reflect"
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
	// local link, and "" while no path is selected.
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
	Now      func() time.Time
	mu       sync.Mutex
	homes    map[string]Home
	parts    sync.Map
}

// New loads the session table as the node starts (sessions.md section 2).
// saved holds the sessions as the last run left them: the caller reports a
// crash for each that was working, and hands back each that was in takeover.
func New(db *store.Store, key identity.Key, now func() time.Time) (h *Host, saved []Session, err error) {
	sessions, saved, err := loadSessions(db, now)
	if err != nil {
		return nil, nil, err
	}
	return &Host{Store: db, Key: key, Sessions: sessions, Now: now, homes: map[string]Home{}}, saved, nil
}

// Part returns what one unit keeps for this host: one value per type, made
// from its zero value at the first call.
func Part[T any](h *Host) *T {
	part, _ := h.parts.LoadOrStore(reflect.TypeFor[T](), new(T))
	return part.(*T)
}

// Attach makes home the connection to the home with that device id, in place
// of an earlier one. node calls it for each connection it opens to a home, and
// with the local link for a home on this node.
func (h *Host) Attach(device string, home Home) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.homes[device] = home
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

// Command signs payload, the JSON text of a command, with the node's key and
// sends it to the home with that device id (contracts/datachannel.md section
// 5). It returns the home's answer when the command was accepted, the home's
// error as an *Error when it was refused, and home_unreachable when no answer
// came (flows/agent-turn.md F).
func (h *Host) Command(ctx context.Context, device, payload string) (json.RawMessage, error) {
	unreachable := &Error{"home_unreachable", "the home of that group is not connected"}
	home, ok := h.Home(device)
	if !ok {
		return nil, unreachable
	}
	env := h.Key.Sign(payload)
	answer, err := home.Request(ctx, func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	})
	if err != nil {
		return nil, unreachable
	}
	var refused proto.ErrorResponse
	if proto.Decode(answer, &refused) == nil {
		return nil, &Error{refused.Err.Code, refused.Err.Message}
	}
	return answer, nil
}

// Pushes holds the handler of each kind of frame a home pushes to a host, by
// the frame's t: "inbox", "rows", "reset". The file that handles a kind
// registers it from its init. home is the device id of the home.
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
// ("takeover.start", contracts/commands.md section 2). The file that handles
// one registers it from its init.
//
// node calls a handler only for a device of the owner, by: for a command,
// after the envelope checks and the receipt lookup, with the payload as data;
// otherwise with the frame. The result holds the result fields of the answer,
// nil when there are none; an *Error is answered with its code, any other
// error with internal.
var Surface = map[string]func(ctx context.Context, h *Host, from Owner, by string, data json.RawMessage) (any, error){}

// Drivers holds the driver of each CLI by its agent name: "claude", "codex".
// A driver's file registers it from its init.
var Drivers = map[string]Driver{}

// A Driver starts one vendor's CLI (contracts/drivers.md section 1).
type Driver interface {
	// Start starts the structured CLI of a session. The process lives no
	// longer than ctx: when ctx ends, its process tree is killed.
	Start(ctx context.Context, l Launch) (Process, error)
	// Interactive returns the command line that opens the session in a
	// terminal, for a takeover (contracts/drivers.md 1.3), and the session
	// id it names. It fails where the CLI cannot open in a terminal a
	// session that has had no turn.
	Interactive(l Launch) (argv []string, session string, err error)
}

// A Launch is what a driver is told about the session it starts.
type Launch struct {
	Folder       string        // the session's working folder, an absolute path
	Session      string        // the CLI's session or thread id; "" before the first start
	Level        int           // 1, 2 or 3 (contracts/drivers.md section 1)
	Token        string        // the session token of this start, for the MCP shim
	Instructions string        // the text of the session kind (contracts/drivers.md section 4)
	RepoConfig   bool          // load the repository's own AI configuration (R-45)
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

// An EventKind names what a CLI process reported.
type EventKind string

const (
	Ready        EventKind = "ready"         // the CLI has a session: Session is its id, to store for the next start
	Active       EventKind = "active"        // the CLI sent something; a silent turn is one without it (contracts/drivers.md 1.2)
	Asked        EventKind = "asked"         // a permission request: Ask, to be answered with Process.Answer
	Used         EventKind = "used"          // usage figures outside a turn's end: Usage
	TurnEnded    EventKind = "turn_ended"    // the turn ended: Status and Usage
	RateLimited  EventKind = "rate_limited"  // the vendor's rate limit was reached: Resets
	LoginInvalid EventKind = "login_invalid" // sign-in is not usable in this mode (R-56): Text
	NoSession    EventKind = "no_session"    // the CLI cannot resume the session id: Text
)

// An Event is one report of a CLI process. Only the fields its kind names
// are set.
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
	ID       string          // names the request in Process.Answer; new on every attempt
	Tool     string          // what the CLI names
	Category string          // command, file, network or other
	Command  string          // the request as the CLI presented it
	Why      string          // the justification the CLI attached; "" without one
	Input    json.RawMessage // what the digest of the approval record covers (flows/approval.md step 5)
}

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

// Tools holds every tool by name. A tool's file registers it from its init.
var Tools = map[string]Tool{}

// ToolsFor returns the tools the server lists to a caller. A tool it does not
// list to a caller is not called for that caller either.
func ToolsFor(c Caller) map[string]Tool {
	listed := maps.Clone(Tools)
	maps.DeleteFunc(listed, func(_ string, tool Tool) bool { return tool.For != nil && !tool.For(c) })
	return listed
}
