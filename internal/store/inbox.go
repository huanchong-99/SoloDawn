package store

import (
	"database/sql"
	"encoding/json"
)

// Item is an unacknowledged inbox item stored on a group home.
type Item struct {
	ID                   int64
	Group, AI, Host, Why string
	Msg, Task            *string
	Env                  json.RawMessage
	NotBefore, Created   int64
}

const selectInbox = "SELECT id,g,ai,host,why,msg,task,env,not_before,created FROM inbox "

// Enqueue allocates the item id so encode can put it into the stored item JSON
// before the transaction commits.
func (tx *Tx) Enqueue(item Item, encode func(int64) (json.RawMessage, error)) (int64, error) {
	var id int64
	err := tx.QueryRow(`INSERT INTO inbox(g,ai,host,why,msg,task,env,not_before,created)
		VALUES(?,?,?,?,?,?,'',?,?) RETURNING id`,
		item.Group, item.AI, item.Host, item.Why, item.Msg, item.Task, item.NotBefore, item.Created).Scan(&id)
	if err != nil {
		return 0, err
	}
	env, err := encode(id)
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec("UPDATE inbox SET env=? WHERE id=?", string(env), id)
	return id, err
}

// Inbox returns due, unacknowledged items for this host in delivery order.
func (s *Store) Inbox(host string, now int64) ([]Item, error) {
	rows, err := s.db.Query(selectInbox+"WHERE host=? AND not_before<=? ORDER BY id", host, now)
	return collect(rows, err, scanItem)
}

func scanItem(rows *sql.Rows) (i Item, err error) {
	err = rows.Scan(&i.ID, &i.Group, &i.AI, &i.Host, &i.Why, &i.Msg, &i.Task,
		(*[]byte)(&i.Env), &i.NotBefore, &i.Created)
	return i, err
}

// Ack deletes an inbox item when acknowledged by its own host in that group.
func (tx *Tx) Ack(group, host string, id int64) error {
	_, err := tx.Exec("DELETE FROM inbox WHERE g=? AND host=? AND id=?", group, host, id)
	return err
}
