package home

import (
	"log"
	"maps"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// The run loop of a home: what the seam leaves to "whoever runs the home".
// A Loop owns one goroutine per group, and that goroutine owns the group's
// State. It applies the commands, the timers and the events of the group one
// after the other, each in one transaction of the store, and tells the
// connected clients what changed (planning/flows/sync.md, D).
//
// The files of the loop: this one holds the Loop and its entries, run_group.go
// what the goroutine of a group does, run_conn.go a connected client,
// run_push.go what clients are told, run_disk.go what is read from the store.
//
// Who waits for whom, so that nothing waits in a circle:
//
//   - A caller of an entry never waits for a client and holds no lock while
//     it waits for a group (Attach, Sync).
//   - The goroutine of a group waits for the store's writer and for nothing
//     else. It never waits for a client: a frame goes into the queue of its
//     connection. It never waits for another group or for itself: the jobs of
//     a group have no bound, so handing one in never waits.
//   - What runs on the store's writer (a command's function, a Disk, a Write)
//     takes no lock of the loop, starts no transaction and sends nothing.
//   - The goroutine that sends for a connection waits for its client and
//     holds no lock while it does.
//
// The registry lock (Loop.mu) is held only for map operations and for
// putting jobs and frames into queues, which never waits. It is taken before
// the lock of a queue and never the other way round.

// Tables are the four tables a loop runs. NodeTables returns those of a node;
// a test or an acceptance step makes its own.
type Tables struct {
	Commands Table
	Timers   Rules[Timer]
	Events   Rules[Handler]
	After    Rules[Hook]
}

// NodeTables returns the tables that the files of this package fill from
// their init.
func NodeTables() Tables {
	return Tables{Commands: Commands, Timers: Timers, Events: Events, After: After}
}

// LoopConfig is what a loop is started with. The loop calls Logs, Now and
// Report on the goroutines of its groups, and Logs and Now also on the
// caller of Cmd. So none of the three may wait for a lock that is held
// around a call of Attach, Sync, Close or Cmd, and none may call those.
type LoopConfig struct {
	Store *store.Store
	Home  string // this node's device id
	Owner string // the user id of this node's owner
	// Logs returns the node's verified device log of a person, as a value
	// that nothing else reads or writes from then on, or nil when the node
	// holds none. The loop asks for the owner's log for every group.
	Logs   func(user string) *identity.Log
	Tables Tables
	// Now is the home's clock; time.Now when nil.
	Now func() time.Time
	// Report takes every failure that is answered internal or that has no
	// request to answer; it writes to the standard logger when nil.
	Report func(error)
	// Every is how often the timers run. With 0 they run only when Tick is
	// called.
	Every time.Duration
}

// Loop runs the groups a node is home to.
type Loop struct {
	cfg  LoopConfig
	wg   sync.WaitGroup // the goroutines of the groups and of the clock
	stop chan struct{}
	once sync.Once

	mu     sync.Mutex
	closed bool
	groups map[string]*runGroup
	conns  map[*Conn]bool
	views  map[string]runView // what each group that exists shows in groups
}

// StartLoop loads every group from the store and starts its goroutine.
func StartLoop(cfg LoopConfig) (*Loop, error) {
	l := &Loop{cfg: runDefaults(cfg), stop: make(chan struct{}),
		groups: map[string]*runGroup{}, conns: map[*Conn]bool{}, views: map[string]runView{}}
	states, err := l.load()
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	for _, s := range states {
		l.views[s.ID] = l.spawn(s).view()
	}
	l.mu.Unlock()
	if l.cfg.Every > 0 {
		l.wg.Add(1)
		go l.clock()
	}
	return l, nil
}

func runDefaults(cfg LoopConfig) LoopConfig {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Report == nil {
		cfg.Report = func(err error) { log.Printf("home: %v", err) }
	}
	return cfg
}

// clock runs the timers of every group as the time goes.
func (l *Loop) clock() {
	defer l.wg.Done()
	ticker := time.NewTicker(l.cfg.Every)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			l.Tick(l.cfg.Now())
		}
	}
}

// Close stops the loop: every group finishes the jobs it was given, no
// connection is sent anything more, and Close returns when the goroutines of
// the groups have ended. The store is closed after it.
func (l *Loop) Close() {
	l.once.Do(func() { close(l.stop) })
	l.mu.Lock()
	l.closed = true
	for _, g := range l.groups {
		g.jobs.shut(true)
	}
	conns := l.conns
	l.groups, l.conns = map[string]*runGroup{}, map[*Conn]bool{}
	l.mu.Unlock()
	for c := range conns {
		c.leave()
	}
	l.wg.Wait()
}

// spawn starts the goroutine of a group with the connections of the node.
// The caller holds l.mu.
func (l *Loop) spawn(s *State) *runGroup {
	g := &runGroup{loop: l, state: s, jobs: runNewQueue[func()](math.MaxInt), seats: map[*Conn]*runSeat{}, next: runNever}
	for c := range l.conns {
		g.seat(c)
	}
	l.groups[s.ID] = g
	l.wg.Add(1)
	go g.run()
	return g
}

// fresh is the state of a group before its rows: the owner's log is all it
// knows.
func (l *Loop) fresh(group string) *State {
	s := NewState(group, l.cfg.Home, l.cfg.Owner)
	l.adopt(s)
	return s
}

// adopt gives a state the owner's log as the node holds it now.
func (l *Loop) adopt(s *State) {
	if log := l.cfg.Logs(l.cfg.Owner); log != nil {
		s.Logs[l.cfg.Owner] = log
	}
}

// route hands a job to the goroutine of a group, which it starts for a
// group that has none: the group of a group create. Such a goroutine ends
// when its jobs are done and the group still does not exist (retire).
func (l *Loop) route(group string, job func(g *runGroup)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	g := l.groups[group]
	if g == nil {
		g = l.spawn(NewState(group, l.cfg.Home, l.cfg.Owner))
		g.jobs.push(func() { l.adopt(g.state) }) // not under l.mu: Logs is the node's code
	}
	g.jobs.push(func() { job(g) })
}

// retire ends the goroutine of a group that does not exist once it has no
// job left. Jobs are handed in under l.mu, so none can arrive in between.
func (l *Loop) retire(g *runGroup) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if g.state.Group.ID != "" || !g.jobs.idle() {
		return false
	}
	delete(l.groups, g.state.ID)
	g.jobs.shut(true)
	return true
}

// each hands a job to every group and returns what waits for all of them.
// The caller holds l.mu.
func (l *Loop) each(job func(g *runGroup)) *sync.WaitGroup {
	var done sync.WaitGroup
	for _, g := range l.groups {
		done.Add(1)
		g.jobs.push(func() { job(g); done.Done() })
	}
	return &done
}

// all is each for a caller that does not hold l.mu.
func (l *Loop) all(job func(g *runGroup)) *sync.WaitGroup {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.each(job)
}

// ask runs f on the goroutine of a group and waits for it. It reports false,
// and runs nothing, for a group that has no goroutine. It must not be called
// on the goroutine of a group or inside a transaction of the store.
func (l *Loop) ask(group string, f func(g *runGroup)) bool {
	done := make(chan struct{})
	l.mu.Lock()
	g := l.groups[group]
	if g != nil {
		g.jobs.push(func() { f(g); close(done) })
	}
	l.mu.Unlock()
	if g == nil {
		return false
	}
	<-done
	return true
}

// Attach makes client a connection of the device it was authenticated for,
// from now until Detach, until the client is done or until the loop gives it
// up (Conn.Gone). When Attach returns, every group counts the device as
// online and Groups is right for it. Attach waits for the groups: it must
// not be called from a hook, a rule or a transaction of the store.
func (l *Loop) Attach(device string, client Client) *Conn {
	c := &Conn{loop: l, device: device, client: client, gone: make(chan struct{}), frames: runNewQueue[any](runBacklog)}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		c.leave()
		return c
	}
	l.conns[c] = true
	seen := l.each(func(g *runGroup) { g.attach(c) })
	l.mu.Unlock()
	go c.pump()
	go c.watch()
	seen.Wait()
	return c
}

// Detach ends a connection: nothing more is sent to it, and every group
// forgets it. It does not close the client. Detaching twice is harmless.
// Every group has the job of forgetting the connection before Gone is
// closed, so what is handed in after Gone finds the device gone.
func (l *Loop) Detach(c *Conn) {
	l.mu.Lock()
	delete(l.conns, c)
	l.each(func(g *runGroup) { g.detach(c) })
	l.mu.Unlock()
	c.leave()
}

// Welcome sends c the answer to its hello: the frame that build makes from
// the groups homed here that the device of c may read, as welcome lists them
// (planning/contracts/datachannel.md section 3). The list is read and the
// frame is queued in one step, and c is pushed its groups only from then on:
// so no groups push comes before the welcome, and none that is older than
// the welcome comes after it. A node calls it for every hello.
//
// build runs under the lock of the loop. It only puts the frame together
// from the list and from values the node worked out before it called
// Welcome, such as time and addrs: it takes no lock, waits for nothing and
// calls nothing, the loop least of all. A build that waits for a lock of the
// node which another goroutine holds while it calls Cmd waits for ever.
func (l *Loop) Welcome(c *Conn, build func(groups []proto.GroupSummary) any) {
	l.mu.Lock()
	c.welcomed = true
	fits := c.frames.push(build(l.listed(c.device)))
	l.mu.Unlock()
	if !fits {
		c.fail()
	}
}

// Cmd hands in the cmd request re that arrived on c with the envelope env
// (planning/contracts/datachannel.md section 5). It returns at once: the
// answer is sent to c when the group has applied the command, after the
// answers to the commands that c sent before it for the same group.
func (l *Loop) Cmd(c *Conn, re int64, env proto.Envelope) {
	group := runGroupOf(env)
	if group == "" {
		l.stray(c, re, env)
		return
	}
	l.route(group, func(g *runGroup) { g.command(c, re, env) })
}

// runNowhere is the id of the state that answers a command without a g. It
// is no random id, so no payload names it.
const runNowhere = "-"

// runGroupOf reads the group a command is for, before any check: it only
// chooses the goroutine. It is "" for a payload that has no g or that the
// envelope check will refuse as bad_request.
func runGroupOf(env proto.Envelope) string {
	var p struct {
		Group proto.ID `json:"g,omitempty"`
	}
	proto.Decode([]byte(env.Payload), &p) // a payload that does not decode leaves ""
	return string(p.Group)
}

// stray answers a command that names no group with the first check that
// fails, in the order of every command: the signer, the envelope, then g. No
// group runs it and it leaves no receipt. Only the owner's devices are known
// to it.
func (l *Loop) stray(c *Conn, re int64, env proto.Envelope) {
	res := refused(&Refusal{"bad_request", "the command names no group in g"})
	if _, refusal := l.fresh(runNowhere).open(c.device, env, l.cfg.Now()); refusal != nil {
		res = refused(refusal)
	}
	c.send(res.Frame(re))
}

// Inbox makes c the connection that takes the inbox items of its device, in
// every group: the items that no host has acknowledged are sent to it now,
// again if they were sent before, and new ones as they are queued. A node
// calls it on every hello of a host (planning/contracts/datachannel.md
// section 6).
func (l *Loop) Inbox(c *Conn) {
	l.mu.Lock()
	c.taker = true
	l.each(func(g *runGroup) { g.inbox(c) })
	l.mu.Unlock()
}

// Groups returns the groups homed here that the device of c may read, as
// they are now. It is a read for tests and tools: a frame built from it can
// be older than a groups push that is already on its way, which is why a
// node answers a hello through Welcome.
func (l *Loop) Groups(c *Conn) []proto.GroupSummary {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.listed(c.device)
}

// listed is the list of groups for a device, in the order of their ids. The
// caller holds l.mu.
func (l *Loop) listed(device string) []proto.GroupSummary {
	list := []proto.GroupSummary{}
	for _, id := range slices.Sorted(maps.Keys(l.views)) {
		if v := l.views[id]; v.serves[device] {
			list = append(list, v.summary)
		}
	}
	return list
}

// Tick runs every timer of the table for every group, at the home's time
// now, and sends the inbox items whose hold has ended.
func (l *Loop) Tick(now time.Time) {
	l.all(func(g *runGroup) { g.tick(now) })
}

// Event runs the handler under that name for every group, with the event as
// its unit defines it; the handler leaves a group that the event does not
// concern alone. It reports false, and runs nothing, when the table has no
// such handler.
func (l *Loop) Event(name string, event any) bool {
	handle, ok := l.cfg.Tables.Events[name]
	if !ok {
		return false
	}
	l.all(func(g *runGroup) {
		g.rule(l.cfg.Now(), func(d Disk, now int64) Result { return handle(g.state, d, now, event) })
	})
	return true
}
