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

func TestGroupExportTransfer(t *testing.T) {
	source := openDB(t, ":memory:")
	target := openDB(t, ":memory:")
	check(t, source.Write(seedExport))
	path := filepath.Join(t.TempDir(), "group %#.sqlite")
	check(t, source.ExportGroup("a", path, ""))
	id, err := ExportedGroup(path)
	check(t, err)
	equal(t, id, "a")
	verifyExportFile(t, path)
	equal(t, scalar[int](t, source.db, "SELECT count(*) FROM row WHERE g='a'"), 6)
	load := func(*Tx) error { return nil }
	equal(t, target.ImportGroup("a", path, "other", "P2", load).Error(), "the node does not belong to the group's owner")
	equal(t, target.ImportGroup("b", path, "owner", "P2", load).Error(), "the file does not contain the expected group")
	equal(t, target.ImportGroup("a", path, "owner", "P2", func(*Tx) error { return errors.New("load failed") }).Error(), "load failed")
	equal(t, scalar[int](t, target.db, "SELECT count(*) FROM row"), 0)

	check(t, source.ExportGroup("a", path, "P2"))
	equal(t, scalar[string](t, source.db, "SELECT value FROM setting WHERE key='moved'"), `{"a":"P2"}`)
	check(t, source.Write(func(tx *Tx) error {
		raw, err := tx.MovedGroups()
		equal(t, string(raw), `{"a":"P2"}`)
		return err
	}))
	equal(t, scalar[int](t, source.db, "SELECT count(*) FROM row"), 1)
	equal(t, scalar[int](t, source.db, "SELECT count(*) FROM receipt"), 1)
	equal(t, scalar[int](t, source.db, "SELECT count(*) FROM kb"), 0)
	_, err = target.db.Exec("INSERT INTO inbox(id,g,ai,host,why,env,created) VALUES(9,'b','ai','host','msg','{}',0)")
	check(t, err)
	check(t, target.ImportGroup("a", path, "owner", "P2", load))
	verifyImportedGroup(t, target)
	equal(t, target.ImportGroup("a", path, "owner", "P2", load).Error(), "the node already holds rows of that group")
	check(t, source.ImportGroup("a", path, "owner", "P1", load))
	equal(t, scalar[string](t, source.db, "SELECT value FROM setting WHERE key='moved'"), "{}")
}

func seedExport(tx *Tx) error {
	_, err := tx.Exec(`
		INSERT INTO grp VALUES ('a',7,12,1),('b',1,0,0);
		INSERT INTO row(g,r,id,seq,pos,ts,mt,gone,data) VALUES
		('a','group','a',1,1,0,0,0,'{"r":"group","home":"P1"}'),
		('a','member','m1',2,2,0,0,0,'{"kind":"human","role":"owner","user":"owner"}'),
		('a','member','m2',3,3,0,0,0,'{"kind":"human","role":"member","user":"person"}'),
		('a','note','n1',5,4,0,0,1,'{"r":"note","gone":true}'),
		('a','note','n2',7,7,0,0,0,'{"r":"note","type":"kb","name":"Plan","text":"Notes"}'),
		('a','device','d1',6,6,0,0,0,'{"r":"device","user":"person","ver":2,"env":{}}'),
		('b','member','m3',1,1,0,0,0,'{"kind":"human","user":"other"}');
		INSERT INTO device_log VALUES ('owner',1,'{}'),('person',1,'{}'),('other',1,'{}');
		INSERT INTO push_sub VALUES ('owner-dev','owner','{}','{}','[]'),('other-dev','other','{}','{}','[]');
		INSERT INTO inbox(g,ai,host,why,env,created) VALUES ('a','ai','host','msg','{"item":1}',0),
			('b','ai','host','msg','{"item":2}',0);
		INSERT INTO receipt VALUES ('device','nonce',0,'{}');
		INSERT INTO kb(g,id,name,text) VALUES ('a','n2','Plan','Notes');`)
	return err
}

func verifyExportFile(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", transferURI(path, "ro"))
	check(t, err)
	defer db.Close()
	equal(t, scalar[string](t, db, "SELECT group_concat(name,',' ORDER BY name) FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%'"),
		"device_log,grp,inbox,push_sub,row")
	equal(t, scalar[int](t, db, "SELECT count(*) FROM row"), 6)
	equal(t, scalar[int](t, db, "SELECT count(*) FROM row WHERE gone=1"), 1)
	equal(t, scalar[int](t, db, "SELECT count(*) FROM inbox"), 1)
	equal(t, scalar[string](t, db, "SELECT group_concat(user,',' ORDER BY user,ver) FROM device_log"), "owner,person")
	equal(t, scalar[string](t, db, "SELECT user FROM push_sub"), "owner")
}

func verifyImportedGroup(t *testing.T, destination *Store) {
	t.Helper()
	equal(t, scalar[string](t, destination.db, "SELECT data->>'home' FROM row WHERE r='group'"), "P2")
	equal(t, scalar[int](t, destination.db, "SELECT purged FROM grp"), 1)
	equal(t, scalar[string](t, destination.db, "SELECT text FROM kb WHERE kb MATCH 'Notes'"), "Notes")
	items, err := destination.Inbox("host", 0)
	check(t, err)
	equal(t, items[1].ID, int64(10))
	equal(t, items[1].Env, json.RawMessage(`{"item":10}`))
	check(t, destination.Write(func(tx *Tx) error {
		next, err := tx.NextNum("a")
		if err != nil || next != 13 {
			return fmt.Errorf("next task number = %d, %v; want 13", next, err)
		}
		return tx.Ack("a", "host", 10)
	}))
	equal(t, scalar[int](t, destination.db, "SELECT count(*) FROM inbox"), 1)
}

func TestExportFailureDoesNotMove(t *testing.T) {
	source := openDB(t, ":memory:")
	check(t, source.Write(seedExport))
	dir := t.TempDir()
	if err := source.ExportGroup("a", dir, "P2"); err == nil {
		t.Fatal("export replaced a directory")
	}
	equal(t, scalar[int](t, source.db, "SELECT count(*) FROM row WHERE g='a'"), 6)
	equal(t, scalar[int](t, source.db, "SELECT count(*) FROM setting"), 0)
	if err := source.ExportGroup("missing", filepath.Join(dir, "export.db"), ""); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("export absent group: %v", err)
	}
	if _, err := ExportedGroup(filepath.Join(dir, "export.db")); err == nil {
		t.Fatal("a failed export left a readable file")
	}
}

func TestGroupTransferDatabaseErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "export.sqlite")
	source := openDB(t, filepath.Join(dir, "node.db"))
	check(t, source.Write(seedExport))
	transferError(t, source.ExportGroup("a", filepath.Join(dir, "absent", "file"), ""), "absent")
	check(t, source.ExportGroup("a", path, ""))
	target := openDB(t, ":memory:")
	load := func(*Tx) error { return nil }
	transferError(t, target.ImportGroup("a", path+".missing", "owner", "P2", load), "unable to open database")
	_, err := target.db.Exec(`CREATE TRIGGER refuse_row BEFORE INSERT ON row BEGIN SELECT raise(ABORT,'refused row'); END`)
	check(t, err)
	transferError(t, target.ImportGroup("a", path, "owner", "P2", load), "refused row")
	_, err = target.db.Exec("DROP TRIGGER refuse_row; DROP TABLE grp")
	check(t, err)
	transferError(t, target.ImportGroup("a", path, "owner", "P2", load), "main.grp")
	_, err = target.db.Exec("DROP TABLE row")
	check(t, err)
	transferError(t, target.ImportGroup("a", path, "owner", "P2", load), "main.row")

	_, err = source.db.Exec("DROP TABLE kb")
	check(t, err)
	transferError(t, source.ExportGroup("a", path, "P2"), "no such table: kb")
	equal(t, scalar[int](t, source.db, "SELECT count(*) FROM row WHERE g='a'"), 6)
	_, err = source.db.Exec("DROP TABLE inbox")
	check(t, err)
	transferError(t, source.ExportGroup("a", path, ""), "no such table: inbox")
	check(t, source.db.Close())
	transferError(t, source.ExportGroup("a", path, ""), "database is closed")
	transferError(t, source.ImportGroup("a", path, "owner", "P2", load), "database is closed")
}

func TestExportRejectsTransferWriteErrors(t *testing.T) {
	source := openDB(t, ":memory:")
	target := openDB(t, ":memory:")
	check(t, source.Write(seedExport))
	check(t, source.Write(func(from *Tx) error {
		return target.Write(func(to *Tx) error {
			transferError(t, from.exportTables(to, "CREATE TABLE row(value TEXT)", "a"), "already exists")
			_, err := to.Exec("DROP TABLE row; CREATE TABLE row(value TEXT)")
			if err != nil {
				return err
			}
			transferError(t, from.exportTable(to, "row", "a"), "1 columns")
			return nil
		})
	}))
}

func transferError(t *testing.T, got error, want string) {
	t.Helper()
	if got == nil || !strings.Contains(got.Error(), want) {
		t.Errorf("error = %v; want %q", got, want)
	}
}
