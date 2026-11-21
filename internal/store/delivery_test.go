package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func signed(id int64) (json.RawMessage, error) {
	return json.RawMessage(fmt.Sprintf(`{"p":"item %d","k":"home","s":"sig"}`, id)), nil
}

func TestInboxDelivery(t *testing.T) {
	s := openDB(t, ":memory:")
	msg, task, commit := "msg", "task", "commit"
	review := Item{Group: "g", AI: "ai", Host: "host", Why: "review", Msg: &msg, Task: &task, Commit: &commit,
		NotBefore: 50, Created: 10}
	notice := Item{Group: "g", AI: "secretary", Host: "host", Why: "notice", Created: 20}
	check(t, s.Write(func(tx *Tx) error {
		for i, item := range []Item{review, {Group: "g", Host: "other"}, notice} {
			id, err := tx.Enqueue(item, signed)
			if err != nil {
				return err
			}
			if id != int64(i+1) {
				return fmt.Errorf("enqueue id %d, want %d", id, i+1)
			}
		}
		return nil
	}))
	review.ID = 1
	review.Env, _ = signed(1)
	notice.ID = 3
	notice.Env, _ = signed(3)
	items, err := s.Inbox("host", 49)
	check(t, err)
	equal(t, items, []Item{notice})
	items, err = s.Inbox("host", 50)
	check(t, err)
	equal(t, items, []Item{review, notice})
	check(t, s.Write(func(tx *Tx) error {
		if err := tx.Ack("g", "other", 1); err != nil {
			return err
		}
		return tx.Ack("not-g", "host", 1)
	}))
	items, err = s.Inbox("host", 51)
	check(t, err)
	equal(t, items, []Item{review, notice})
	check(t, s.Write(func(tx *Tx) error { return tx.Ack("g", "host", 1) }))
	items, err = s.Inbox("host", 51)
	check(t, err)
	equal(t, items, []Item{notice})
	stop := errors.New("signer unavailable")
	err = s.Write(func(tx *Tx) error {
		_, err := tx.Enqueue(notice, func(int64) (json.RawMessage, error) { return nil, stop })
		return err
	})
	equal(t, err, stop)
	items, err = s.Inbox("host", 51)
	check(t, err)
	equal(t, items, []Item{notice})
}

func TestReceipts(t *testing.T) {
	s := openDB(t, ":memory:")
	no := false
	out := Outcome{OK: true, Num: 12, Round: 1, Passed: &no}
	denied := Outcome{Code: "forbidden"}
	check(t, s.Write(func(tx *Tx) error {
		if err := tx.PutReceipt("a", "n", 40, out); err != nil {
			return err
		}
		return tx.PutReceipt("b", "n", 50, denied)
	}))
	equal(t, scalar[string](t, s.db, "SELECT answer FROM receipt WHERE k='a'"), `{"ok":true,"num":12,"passed":false,"round":1}`)
	equal(t, scalar[string](t, s.db, "SELECT answer FROM receipt WHERE k='b'"), `{"code":"forbidden"}`)
	err := s.Write(func(tx *Tx) error { return tx.PutReceipt("a", "n", 60, denied) })
	if err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed: receipt.k, receipt.n") {
		t.Fatalf("duplicate receipt: %v", err)
	}
	for _, test := range []struct {
		k, n string
		want Outcome
		err  error
	}{{"a", "n", out, nil}, {"b", "n", denied, nil}, {"a", "other", Outcome{}, sql.ErrNoRows}} {
		var got Outcome
		err := s.Write(func(tx *Tx) error { var err error; got, err = tx.Receipt(test.k, test.n); return err })
		equal(t, got, test.want)
		equal(t, err, test.err)
	}
}

func TestInboxFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.db")
	s := openDB(t, path)
	unlock := lockDB(t, path)
	called := false
	err := s.Write(func(tx *Tx) error {
		id, err := tx.Enqueue(Item{Group: "g", Host: "host", Why: "notice"}, func(int64) (json.RawMessage, error) {
			called = true
			return nil, nil
		})
		equal(t, id, int64(0))
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "database is locked") {
		t.Fatalf("locked enqueue: %v", err)
	}
	equal(t, called, false)
	unlock()

	closed, err := Open(":memory:")
	check(t, err)
	check(t, closed.Close())
	items, err := closed.Inbox("host", 0)
	equal(t, items, ([]Item)(nil))
	equal(t, err.Error(), "sql: database is closed")
}
