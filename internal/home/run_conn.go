package home

import (
	"io"
	"sync"
	"sync/atomic"
)

// runBacklog is how many frames may wait for one connection. A frame is at
// most 60 KiB (planning/contracts/conventions.md section 4).
const runBacklog = 1024

// Conn is a client attached to a loop: the device it was authenticated for
// and the frames that wait for it. The loop never sends on the goroutine of
// a group: a frame goes into the queue of its connection, and one goroutine
// per connection hands the frames to the client in order.
//
// A client's rows must have no gap, so no frame is ever left out. A
// connection whose client refuses a frame, or for which more than runBacklog
// frames wait, is given up instead: nothing more is sent to it, every group
// forgets it, and its client is closed when it has a Close method, as a
// peer.Connection has. The client connects again and reads again with sync
// (planning/contracts/datachannel.md section 3).
type Conn struct {
	loop   *Loop
	device string
	client Client
	frames *runQueue[any]
	gone   chan struct{}
	once   sync.Once
	broken atomic.Bool // the loop gave the connection up
	// welcomed says that the connection has had its welcome and is pushed
	// its groups from then on. The lock of the loop guards it.
	welcomed bool
	// taker says that the connection takes the inbox items of its device; the lock of the loop guards it.
	taker bool
}

// Gone is closed when the loop sends nothing more to the connection: after
// Detach, after its client is done, after the loop gave it up, and after
// Close of the loop.
func (c *Conn) Gone() <-chan struct{} { return c.gone }

// send queues a frame for the client. It never waits.
func (c *Conn) send(frame any) {
	if !c.frames.push(frame) {
		c.fail()
	}
}

// fail gives the connection up.
func (c *Conn) fail() {
	c.broken.Store(true)
	c.loop.Detach(c)
}

// leave ends the sending; what still waits is dropped.
func (c *Conn) leave() {
	c.once.Do(func() {
		close(c.gone)
		c.frames.shut(false)
	})
}

// pump hands the queued frames to the client, one at a time.
func (c *Conn) pump() {
	for {
		frame, ok := c.frames.pop()
		if !ok {
			return
		}
		if c.client.Send(frame) != nil {
			c.fail()
		}
	}
}

// watch detaches the connection when its client is done, and closes the
// client of a connection that the loop gave up.
func (c *Conn) watch() {
	select {
	case <-c.client.Done():
		c.loop.Detach(c)
	case <-c.gone:
	}
	if closer, ok := c.client.(io.Closer); ok && c.broken.Load() {
		closer.Close()
	}
}

// runQueue is a list of values in the order they were put in, for one
// reader. Putting one in never waits.
type runQueue[T any] struct {
	mu     sync.Mutex
	filled sync.Cond // signalled when a value was put in or the queue was shut
	items  []T
	limit  int
	closed bool
}

func runNewQueue[T any](limit int) *runQueue[T] {
	q := &runQueue[T]{limit: limit}
	q.filled.L = &q.mu
	return q
}

// push puts a value in. It reports false when limit values wait already. A
// queue that is shut takes the value and drops it.
func (q *runQueue[T]) push(v T) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return true
	}
	if len(q.items) >= q.limit {
		return false
	}
	q.items = append(q.items, v)
	q.filled.Signal()
	return true
}

// pop waits for the next value. ok is false when the queue is shut and
// nothing is left in it.
func (q *runQueue[T]) pop() (v T, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.items) == 0 && !q.closed {
		q.filled.Wait()
	}
	if len(q.items) == 0 {
		return v, false
	}
	v, q.items = q.items[0], q.items[1:]
	return v, true
}

// idle reports whether nothing waits.
func (q *runQueue[T]) idle() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) == 0
}

// shut ends the queue: nothing more goes in. With keep the reader still gets
// what waits; without it that is dropped.
func (q *runQueue[T]) shut(keep bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	if !keep {
		q.items = nil
	}
	q.filled.Broadcast()
}
