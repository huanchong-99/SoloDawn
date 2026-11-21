package store

import "encoding/json"

type Item struct {
	ID                   int64
	Group, AI, Host, Why string
	Msg, Task, Commit    *string
	Env, Cause           json.RawMessage
	NotBefore, Created   int64
}

// Enqueue reserves the item id so sign can bind it into the envelope
// before the command commits. Cause is the unmodified signed decision.
func (tx *Tx) Enqueue(item Item, sign func(int64) (json.RawMessage, error)) (int64, error) {
	var id int64
	err := tx.QueryRow(`INSERT INTO inbox(g,ai,host,why,msg,task,commit_id,env,cause,not_before,created)
		VALUES(?,?,?,?,?,?,?,'',cast(? AS text),?,?) RETURNING id`,
		item.Group, item.AI, item.Host, item.Why, item.Msg, item.Task, item.Commit, item.Cause, item.NotBefore, item.Created).Scan(&id)
	if err != nil {
		return 0, err
	}
	env, err := sign(id)
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec("UPDATE inbox SET env=? WHERE id=?", string(env), id)
	return id, err
}

// Inbox returns due, unacknowledged items for this host in delivery order.
func (s *Store) Inbox(host string, now int64) ([]Item, error) {
	rows, err := s.db.Query(`SELECT id,g,ai,host,why,msg,task,commit_id,env,cause,not_before,created
		FROM inbox WHERE host=? AND not_before<=? ORDER BY id`, host, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		var i Item
		// A Scan failure surfaces through rows.Err() on the next line, because inbox is STRICT and the driver reads every column before Next returns true.
		_ = rows.Scan(&i.ID, &i.Group, &i.AI, &i.Host, &i.Why, &i.Msg, &i.Task, &i.Commit,
			(*[]byte)(&i.Env), (*[]byte)(&i.Cause), &i.NotBefore, &i.Created)
		items = append(items, i)
	}
	return items, rows.Err()
}

func (tx *Tx) Ack(group, host string, id int64) error {
	_, err := tx.Exec("DELETE FROM inbox WHERE g=? AND host=? AND id=?", group, host, id)
	return err
}
