package store

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
)

// rc1Node is what a node of v3.0.0-rc.1 holds: a project group homed here with
// a person from elsewhere, an AI of this host in it, and the node's own data.
const rc1Node = `
INSERT INTO setting VALUES ('owner','"u-owner"'),('name','"studio"'),('homes','[{"home":"d-home"}]'),
  ('moved','{"g-old":"d-laptop"}'),('approvers','["u-owner"]'),('default_level','3'),('max_open','4'),
  ('idle_minutes','15'),('offline','false'),('retention_days','90');
INSERT INTO row(g,r,id,seq,pos,ts,mt,keep,data) VALUES
  ('g1','group','g1',1,1,1,1,0,'{"r":"group","id":"g1","kind":"project","repo":"git@example.com:a/b.git","base":"main","forge":"github","checks":["go test ./..."]}'),
  ('g1','member','m-ai',2,2,1,1,0,'{"r":"member","id":"m-ai","kind":"ai","cli":"claude","host":"d-owner-mac"}'),
  ('g1','member','m-friend',3,3,1,1,0,'{"r":"member","id":"m-friend","kind":"human","user":"u-friend"}'),
  ('g1','task','t1',4,4,1,1,0,'{"r":"task","id":"t1","state":"merged","branch":"solodawn/t-1","pr":12,"commit":"9f2c1ab"}'),
  ('g1','note','k1',5,5,1,1,1,'{"r":"note","id":"k1","type":"kb","name":"Plan","text":"ship"}');
INSERT INTO grp VALUES ('g1',5,1,0);
INSERT INTO inbox(id,g,ai,host,why,msg,task,commit_id,env,created) VALUES (7,'g1','m-ai','d-owner-mac','review','r1','t1','9f2c1ab','{"item":7}',1);
INSERT INTO kb(g,id,name,text) VALUES ('g1','k1','Plan','ship');
INSERT INTO perm VALUES ('claude','g1',2);
INSERT INTO session(g,member,task,kind,folder,agent,home,name,cli_session,state,takeover_at)
  VALUES ('g1','m-ai','t1','task','/data/wt/g1/t-1','claude','d-home','T-1','c1','dormant','9f2c1ab');
INSERT INTO approval VALUES ('card1','g1','m-ai','t1','Bash','sha','pending');
INSERT INTO device_log VALUES ('u-owner',1,'{}'),('u-owner',2,'{}'),('u-friend',1,'{}');
INSERT INTO push_sub VALUES ('d-owner-phone','u-owner','{}','{}','[]'),('d-friend-phone','u-friend','{}','{}','[]');
INSERT INTO receipt VALUES ('d-owner-phone','n1',1,'{"ok":true}');
INSERT INTO agent(agent,cli_version,account) VALUES ('claude','2.1.0','team');
INSERT INTO audit(ts,g,agent,what,item,home,detail) VALUES (1,'g1','claude','delivered',7,'d-home','');
INSERT INTO usage VALUES ('claude','2026-11-20','{}');
INSERT INTO peer VALUES ('d-laptop','wss://rv.example',1);
INSERT INTO processed_offer VALUES ('d-laptop','c1',99);`

func TestMigration4ForgetsTheGroupsOfRC1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.db")
	db, err := sql.Open("sqlite", path)
	check(t, err)
	check(t, migrate(db, migrations[:3]))
	_, err = db.Exec(rc1Node)
	check(t, err)
	check(t, db.Close())

	s := openDB(t, path)
	equal(t, scalar[int](t, s.db, "PRAGMA user_version"), 4)
	for _, table := range []string{"row", "grp", "inbox", "kb", "perm", "session", "approval"} {
		equal(t, scalar[int](t, s.db, "SELECT count(*) FROM "+table), 0)
	}
	equal(t, scalar[string](t, s.db, "SELECT group_concat(key,',' ORDER BY key) FROM setting"),
		"approvers,default_level,idle_minutes,max_open,name,offline,owner,retention_days")
	equal(t, scalar[string](t, s.db, "SELECT group_concat(user||':'||ver,',' ORDER BY ver) FROM device_log"), "u-owner:1,u-owner:2")
	equal(t, scalar[string](t, s.db, "SELECT group_concat(device) FROM push_sub"), "d-owner-phone")
	equal(t, scalar[string](t, s.db, `SELECT (SELECT n FROM receipt)||(SELECT account FROM agent)||(SELECT item FROM audit)||
		(SELECT day FROM usage)||(SELECT url FROM peer)||(SELECT conn FROM processed_offer)`), "n1team72026-11-20wss://rv.examplec1")
	equal(t, columns(t, s, "session"), "g,member,task,kind,folder,agent,home,name,secretary,cli_session,state,last_active,report")
	equal(t, columns(t, s, "inbox"), "id,g,ai,host,why,msg,task,commit_id,env,not_before,created")
	check(t, s.Write(func(tx *Tx) error {
		id, err := tx.Enqueue(Item{Group: "g2", AI: "m", Host: "d-owner-mac", Why: "notice"},
			func(int64) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
		equal(t, id, int64(8))
		return err
	}))
}

func TestMigration4WithoutAnOwnerForgetsEveryDevice(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	check(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	check(t, migrate(db, migrations[:3]))
	_, err = db.Exec("INSERT INTO device_log VALUES ('u-friend',1,'{}'); INSERT INTO push_sub VALUES ('d','u-friend','{}','{}','[]')")
	check(t, err)
	check(t, migrate(db, migrations))
	equal(t, scalar[int](t, db, "SELECT (SELECT count(*) FROM device_log)+(SELECT count(*) FROM push_sub)"), 0)
}

func columns(t *testing.T, s *Store, table string) string {
	t.Helper()
	return scalar[string](t, s.db, "SELECT group_concat(name,',' ORDER BY cid) FROM pragma_table_info(?)", table)
}
