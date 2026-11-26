package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const transferOwner = `SELECT data->>'user' FROM %s.row
	WHERE g=? AND r='member' AND gone=0 AND data->>'role'='owner'`

func (s *Store) transfer(file string, run func(*Tx) error) error {
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "ATTACH DATABASE ? AS transfer", file); err != nil {
		return err
	}
	defer conn.ExecContext(ctx, "DETACH DATABASE transfer")
	return transaction(conn, run)
}

// ExportGroup writes a copy, and records the destination when it is a move.
func (s *Store) ExportGroup(group, file, to string) error {
	tmp, err := os.CreateTemp(filepath.Dir(file), ".solodawn-export-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	tmp.Close()
	publish := func(tx *Tx) error {
		if err := tx.exportCopy(group, name); err != nil {
			return err
		}
		return os.Rename(name, file)
	}
	if to == "" {
		view, err := s.Snapshot(group)
		if err != nil {
			return err
		}
		defer view.Close()
		return publish(view.tx)
	}
	return s.Write(func(tx *Tx) error {
		if err := publish(tx); err != nil {
			return err
		}
		return tx.moveGroup(group, to)
	})
}

func (tx *Tx) exportCopy(group, file string) error {
	var owner string
	if err := tx.QueryRow(fmt.Sprintf(transferOwner, "main"), group).Scan(&owner); err != nil {
		return fmt.Errorf("group is not homed here: %w", err)
	}
	ddl, _, _ := strings.Cut(schema, "CREATE TABLE receipt")
	ddl += fmt.Sprintf("PRAGMA user_version=%d;", len(migrations))
	dst, _ := sql.Open("sqlite", transferURI(file, "rw"))
	defer dst.Close()
	return transaction(dst, func(out *Tx) error { return tx.exportTables(out, ddl, group) })
}

func (tx *Tx) exportTables(out *Tx, ddl, group string) error {
	if _, err := out.Exec(ddl); err != nil {
		return err
	}
	for _, table := range []string{"row", "grp", "inbox", "device_log", "push_sub"} {
		if err := tx.exportTable(out, table, group); err != nil {
			return err
		}
	}
	return nil
}

func (tx *Tx) exportTable(out *Tx, table, group string) error {
	rows, err := tx.Query(exportQuery(table), group)
	if err != nil {
		return err
	}
	defer rows.Close()
	return exportRows(out, table, rows)
}

const exportPeople = `(SELECT data->>'user' FROM row
	WHERE g=?1 AND r='member' AND gone=0 AND data->>'kind'='human')`

func exportQuery(table string) string {
	switch table {
	case "device_log", "push_sub":
		return "SELECT * FROM " + table + " WHERE user IN " + exportPeople
	default:
		return "SELECT * FROM " + table + " WHERE g=?"
	}
}

func exportRows(out *Tx, table string, rows *sql.Rows) error {
	cols, _ := rows.Columns()
	args := make([]any, len(cols))
	values := make([]any, len(cols))
	for i := range args {
		args[i] = &values[i]
	}
	insert := "INSERT INTO " + table + " VALUES (" + strings.Repeat("?,", len(cols)-1) + "?)"
	for rows.Next() {
		err := rows.Scan(args...)
		if err == nil {
			_, err = out.Exec(insert, values...)
		}
		if err != nil {
			return err
		}
	}
	return rows.Err()
}

func (tx *Tx) moveGroup(group, to string) error {
	for _, table := range []string{"row", "grp", "inbox", "kb"} {
		if _, err := tx.Exec("DELETE FROM "+table+" WHERE g=?", group); err != nil {
			return err
		}
	}
	_, err := tx.Exec(`INSERT INTO setting(key,value) VALUES('moved',json_object(?,?))
		ON CONFLICT(key) DO UPDATE SET value=json_patch(value,excluded.value)`, group, to)
	return err
}

// MovedGroups returns the destinations recorded by completed exports.
func (tx *Tx) MovedGroups() ([]byte, error) {
	var raw []byte
	err := tx.QueryRow("SELECT coalesce((SELECT value FROM setting WHERE key='moved'),'{}')").Scan(&raw)
	return raw, err
}

func transferURI(file, mode string) string {
	return "file:" + url.PathEscape(filepath.ToSlash(file)) + "?mode=" + mode
}

// ExportedGroup reads the lone group ID in the export file.
func ExportedGroup(file string) (string, error) {
	db, _ := sql.Open("sqlite", transferURI(file, "ro"))
	defer db.Close()
	if err := checkVersion(db, "main"); err != nil {
		return "", err
	}
	return exportedGroup(db, "main")
}

type querier interface {
	QueryRow(string, ...any) *sql.Row
}

// checkVersion refuses a file written by a node of another schema version; a
// file of v3.0.0-rc.1 has 0.
func checkVersion(db querier, schema string) error {
	var version int
	if err := db.QueryRow("PRAGMA " + schema + ".user_version").Scan(&version); err != nil {
		return err
	}
	if version != len(migrations) {
		return fmt.Errorf("the file was written by another version of SoloDawn (file version %d, this node %d)", version, len(migrations))
	}
	return nil
}

func exportedGroup(db querier, schema string) (string, error) {
	var group string
	err := db.QueryRow("SELECT min(g) FROM " + schema + ".grp HAVING count(*)=1").Scan(&group)
	return group, err
}

// ImportGroup installs an export and lets the home load its state before committing.
func (s *Store) ImportGroup(group, file, owner, home string, load func(*Tx) error) error {
	return s.onWriter(func() error {
		return s.transfer(transferURI(file, "ro"), func(tx *Tx) error {
			if err := checkVersion(tx, "transfer"); err != nil {
				return err
			}
			if err := tx.checkImport(group, owner); err != nil {
				return err
			}
			if err := tx.importGroup(group, home); err != nil {
				return err
			}
			return load(tx)
		})
	})
}

func (tx *Tx) checkImport(group, owner string) error {
	id, err := exportedGroup(tx, "transfer")
	if err != nil || id != group {
		return errors.New("the file does not contain the expected group")
	}
	var user string
	if err := tx.QueryRow(fmt.Sprintf(transferOwner, "transfer")+" AND data->>'user'<>''", group).Scan(&user); err != nil || user != owner {
		return errors.New("the node does not belong to the group's owner")
	}
	var exists bool
	err = tx.QueryRow("SELECT EXISTS (SELECT 1 FROM main.row WHERE g=?)", group).Scan(&exists)
	if exists {
		return errors.New("the node already holds rows of that group")
	}
	return err
}

func (tx *Tx) importGroup(group, home string) error {
	const copyRows = `INSERT INTO main.row
		SELECT g,r,id,seq,pos,ts,mt,thread,in_main,pending,final,keep,gone,
		CASE WHEN r='group' AND gone=0 THEN json_set(data,'$.home',?) ELSE data END
		FROM transfer.row WHERE g=?`
	if _, err := tx.Exec(copyRows, home, group); err != nil {
		return err
	}
	for _, query := range importQueries {
		if _, err := tx.Exec(query, group); err != nil {
			return err
		}
	}
	return tx.backfillGroupKB(group)
}

var importQueries = []string{
	"INSERT OR REPLACE INTO main.grp SELECT * FROM transfer.grp WHERE g=?",
	`INSERT INTO main.inbox(g,ai,host,why,msg,task,commit_id,env,not_before,created)
		SELECT g,ai,host,why,msg,task,commit_id,env,not_before,created FROM transfer.inbox WHERE g=? ORDER BY id`,
	"UPDATE main.inbox SET env=json_set(env,'$.item',id) WHERE g=?",
	`INSERT OR IGNORE INTO main.device_log SELECT * FROM transfer.device_log WHERE user IN
		(SELECT data->>'user' FROM main.row WHERE g=? AND r='member' AND gone=0 AND data->>'kind'='human')`,
	`INSERT OR IGNORE INTO main.push_sub SELECT * FROM transfer.push_sub WHERE user IN
		(SELECT data->>'user' FROM main.row WHERE g=? AND r='member' AND gone=0 AND data->>'kind'='human')`,
	`UPDATE main.setting SET value=json_remove(value, '$."' || ? || '"') WHERE key='moved'`,
}
