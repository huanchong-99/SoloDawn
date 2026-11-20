package home

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// The parts of the run loop that are rules of arithmetic, pinned to the byte
// and to the millisecond. What the loop does as a whole is tested through
// its entries, in package home_test.

func runSame(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
}

// A frame takes values while they fit with the commas between them, and the
// value that does not fit starts the next frame.
func TestRunBatchesFillAFrameToTheByte(t *testing.T) {
	for _, c := range []struct{ sizes, want [][]int }{
		{nil, nil},
		{[][]int{{10}}, [][]int{{10}}},
		{[][]int{{3, 3, 2}}, [][]int{{3, 3, 2}}},
		{[][]int{{3, 3, 3}}, [][]int{{3, 3}, {3}}},
		{[][]int{{5, 4}}, [][]int{{5, 4}}},
		{[][]int{{5, 5}}, [][]int{{5}, {5}}},
		{[][]int{{10, 10, 1, 8, 1}}, [][]int{{10}, {10}, {1, 8}, {1}}},
	} {
		var list []json.RawMessage
		for _, sizes := range c.sizes {
			for _, size := range sizes {
				list = append(list, bytes.Repeat([]byte("x"), size))
			}
		}
		var got [][]int
		for _, batch := range runBatches(list, 10) {
			var sizes []int
			for _, v := range batch {
				sizes = append(sizes, len(v))
			}
			got = append(got, sizes)
		}
		runSame(t, got, c.want)
	}
}

// An item is due from the millisecond of its hold on; the next hold is the
// earliest of the others.
func TestRunDueSplitsTheQueueAtTheClock(t *testing.T) {
	queue := []store.Item{{ID: 1}, {ID: 2, NotBefore: 99}, {ID: 3, NotBefore: 100}, {ID: 4, NotBefore: 150}, {ID: 5, NotBefore: 101}, {ID: 6, NotBefore: 120}}
	due, next := runDue(queue, 100)
	runSame(t, due, queue[:3])
	runSame(t, next, int64(101))
	due, next = runDue(queue, 150)
	runSame(t, due, queue)
	runSame(t, next, int64(runNever))
	due, next = runDue(nil, 0)
	runSame(t, []any{len(due), next}, []any{0, int64(runNever)})
}

// A queue takes values up to its limit and gives them back in order; shut,
// it takes none, and gives what waits only when asked to keep it.
func TestRunQueueKeepsItsOrderAndItsLimit(t *testing.T) {
	q := runNewQueue[int](2)
	runSame(t, []bool{q.idle(), q.push(1), q.idle(), q.push(2), q.push(3)}, []bool{true, true, false, true, false})
	v, ok := q.pop()
	runSame(t, []any{v, ok, q.push(4), q.push(5)}, []any{1, true, true, false})
	q.shut(true)
	runSame(t, q.push(6), true)
	for _, want := range []int{2, 4} {
		v, ok = q.pop()
		runSame(t, []any{v, ok}, []any{want, true})
	}
	v, ok = q.pop()
	runSame(t, []any{v, ok, q.idle()}, []any{0, false, true})

	q = runNewQueue[int](2)
	q.push(1)
	q.shut(false)
	v, ok = q.pop()
	runSame(t, []any{v, ok}, []any{0, false})

	// A reader waits for a value, and for the end.
	q = runNewQueue[int](2)
	got := make(chan [2]any)
	read := func() {
		v, ok := q.pop()
		got <- [2]any{v, ok}
	}
	go read()
	q.push(7)
	runSame(t, <-got, [2]any{7, true})
	go read()
	q.shut(true)
	runSame(t, <-got, [2]any{0, false})
}

type runTestClient chan any

func (c runTestClient) Send(frame any) error  { c <- frame; return nil }
func (c runTestClient) Done() <-chan struct{} { return nil }

// The goroutine of a group that does not exist ends when it has answered,
// and the loop forgets it.
func TestRunForgetsAGroupThatDoesNotExist(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	l, err := StartLoop(LoopConfig{Store: db, Home: "home", Owner: "owner", Logs: func(string) *identity.Log { return nil }, Tables: NodeTables()})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	client := make(runTestClient, 4)
	c := l.Attach("device", client)
	groups := func() int {
		l.mu.Lock()
		defer l.mu.Unlock()
		return len(l.groups)
	}
	for re := range int64(3) {
		l.Cmd(c, re, proto.Envelope{Payload: `{"g":"CQkJCQkJCQkJCQkJCQkJCQ"}`})
		runSame(t, <-client, any(proto.ErrorResponse{Re: re, Err: proto.WireError{Code: "forbidden", Message: "the command is signed by another device than this connection's"}}))
		for end := time.Now().Add(10 * time.Second); groups() != 0; {
			if time.Now().After(end) {
				t.Fatal("the group's goroutine did not end")
			}
			time.Sleep(time.Millisecond)
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	runSame(t, len(l.views), 0)
}
