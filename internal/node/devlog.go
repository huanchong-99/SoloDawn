package node

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const ctlLimit = 61_440

// Logs holds the verified device logs of the owner and group members.
// Lock order: writeMu -> Store.Write / mu; mu is a leaf lock guarding owner,
// log and onChange, never held across Store.Write, home.Loop or network writes.
type Logs struct {
	db          *store.Store
	self, owner string
	writeMu     sync.Mutex
	mu          sync.RWMutex
	log         map[string]*identity.Log
	onChange    func(user string, rows []proto.DeviceRow, adopted bool, skip *sesConn)
}

// OpenLogs loads the owner setting and verified device logs from db.
func OpenLogs(db *store.Store, selfID string) (*Logs, error) {
	l := &Logs{db: db, self: selfID, log: map[string]*identity.Log{}}
	load := func(tx *store.Tx) error { return l.dvlLoad(tx) }
	if err := db.Write(load); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Logs) dvlLoad(tx *store.Tx) error {
	var raw string
	if err := tx.QueryRow("SELECT value FROM setting WHERE key='owner'").Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if err := json.Unmarshal([]byte(raw), &l.owner); err != nil {
		return err
	}
	return l.dvlLoadEntries(tx)
}

func (l *Logs) dvlLoadEntries(tx *store.Tx) error {
	rows, err := tx.Query("SELECT user, env FROM device_log ORDER BY user, ver")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := l.dvlLoadEntry(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (l *Logs) dvlLoadEntry(rows *sql.Rows) error {
	var user, raw string
	if err := rows.Scan(&user, &raw); err != nil {
		return err
	}
	var env proto.Envelope
	if proto.Decode([]byte(raw), &env) != nil {
		return identity.BadRequest
	}
	log := cmp.Or(l.log[user], &identity.Log{User: user})
	if _, err := log.Apply(env); err != nil {
		return err
	}
	l.log[user] = log
	return nil
}

// Owner returns the user id of this node's owner, or empty before adoption.
func (l *Logs) Owner() string { l.mu.RLock(); defer l.mu.RUnlock(); return l.owner }

// UserOf returns the person whose log contains device, or empty.
func (l *Logs) UserOf(device string) string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, user := range slices.Sorted(maps.Keys(l.log)) {
		if l.log[user].Valid(device) {
			return user
		}
	}
	return ""
}

// IsOwner reports whether device is a currently valid device of this node's owner.
func (l *Logs) IsOwner(device string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	log := l.log[l.owner]
	return log != nil && log.Valid(device)
}

// Known reports whether device is valid in the device log kept by this node.
func (l *Logs) Known(device string) bool { return l.UserOf(device) != "" }

// Log returns an independent copy of user's verified log, or nil when none is held.
func (l *Logs) Log(user string) *identity.Log {
	l.mu.RLock()
	log := l.log[user]
	if log == nil {
		l.mu.RUnlock()
		return nil
	}
	l.mu.RUnlock()
	copied, _ := identity.Replay(user, log.Entries)
	return copied
}

func (l *Logs) frames(user string) []proto.Devlog {
	if log := l.Log(user); log != nil {
		return dvlBatchFrames(user, log.Entries)
	}
	return nil
}

func (l *Logs) dvlReceive(dl proto.Devlog, skip *sesConn) error {
	rows, err := l.dvlCommit(dl.User, dl.Entries)
	if len(rows) > 0 {
		l.dvlNotify(dl.User, rows, false, skip)
	}
	return err
}

func (l *Logs) dvlCommit(user string, entries []proto.Envelope) ([]proto.DeviceRow, error) {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()
	log := l.Log(user)
	if log == nil {
		if user != l.Owner() {
			return nil, nil
		}
		log = &identity.Log{User: user}
	}
	rows := dvlCollectRows(log, entries)
	if len(rows) == 0 {
		return nil, nil
	}
	write := func(tx *store.Tx) error { return dvlPutEntries(tx, user, rows) }
	if err := l.db.Write(write); err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.log[user] = log
	l.mu.Unlock()
	return rows, nil
}

func (l *Logs) dvlKeep(log *identity.Log, skip *sesConn) {
	if rows := l.dvlKeepRows(log); len(rows) != 0 {
		l.dvlNotify(log.User, rows, false, skip)
	}
}

func (l *Logs) dvlKeepRows(log *identity.Log) []proto.DeviceRow {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()
	var rows []proto.DeviceRow
	if prev := l.Log(log.User); prev != nil {
		rows = dvlCollectRows(prev, log.Entries)
		log = prev
	}
	l.mu.Lock()
	l.log[log.User] = log
	l.mu.Unlock()
	return rows
}

func dvlCollectRows(log *identity.Log, entries []proto.Envelope) []proto.DeviceRow {
	var rows []proto.DeviceRow
	for _, env := range entries {
		if added, _ := log.Apply(env); added {
			rows = append(rows, dvlRowOf(env))
		}
	}
	return rows
}

func (l *Logs) dvlAdopt(peer string, cand *identity.Log, skip *sesConn) (bool, error) {
	l.writeMu.Lock()
	if l.Owner() != "" || !cand.Adopts(l.self, peer) {
		l.writeMu.Unlock()
		return false, nil
	}
	rows := make([]proto.DeviceRow, len(cand.Entries))
	for i, env := range cand.Entries {
		rows[i] = dvlRowOf(env)
	}
	err := l.dvlSaveAdoption(cand, rows)
	l.writeMu.Unlock()
	if err == nil {
		l.dvlNotify(cand.User, rows, true, skip)
	}
	return err == nil, err
}

func (l *Logs) dvlSaveAdoption(cand *identity.Log, rows []proto.DeviceRow) error {
	ownerJSON := string(dvlMustEncode(cand.User))
	write := func(tx *store.Tx) error {
		if _, err := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', ?)", ownerJSON); err != nil {
			return err
		}
		return dvlPutEntries(tx, cand.User, rows)
	}
	if err := l.db.Write(write); err != nil {
		return err
	}
	l.mu.Lock()
	l.owner, l.log[cand.User] = cand.User, cand
	l.mu.Unlock()
	return nil
}

func (l *Logs) dvlAppendAdd(signer identity.Key, dev, kind, name string, now int64, skip *sesConn) error {
	l.writeMu.Lock()
	row, err := l.dvlWriteAdd(signer, dev, kind, name, now)
	l.writeMu.Unlock()
	if err == nil {
		l.dvlNotify(row.User, []proto.DeviceRow{row}, false, skip)
	}
	return err
}

func (l *Logs) dvlWriteAdd(signer identity.Key, dev, kind, name string, now int64) (proto.DeviceRow, error) {
	owner := l.Owner()
	log := l.Log(owner)
	if log == nil {
		return proto.DeviceRow{}, errors.New("node has no owner log")
	}
	entry := proto.DeviceEntry{
		PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: proto.NewNonce(), Timestamp: now},
		DeviceChange:  proto.DeviceChange{User: owner, Version: len(log.Entries) + 1, Op: "add", Device: dev, Kind: kind, Name: name},
	}
	env := signer.Sign(string(dvlMustEncode(entry)))
	if _, err := log.Apply(env); err != nil {
		return proto.DeviceRow{}, err
	}
	row := dvlRowOf(env)
	write := func(tx *store.Tx) error { return dvlPutEntries(tx, owner, []proto.DeviceRow{row}) }
	if err := l.db.Write(write); err != nil {
		return proto.DeviceRow{}, err
	}
	l.mu.Lock()
	l.log[owner] = log
	l.mu.Unlock()
	return row, nil
}

func dvlPutEntries(tx *store.Tx, user string, rows []proto.DeviceRow) error {
	for _, r := range rows {
		if _, err := tx.Exec("INSERT INTO device_log(user, ver, env) VALUES(?, ?, ?)", user, r.Version, string(dvlMustEncode(r.Env))); err != nil {
			return err
		}
	}
	return nil
}

func (l *Logs) dvlNotify(user string, rows []proto.DeviceRow, adopted bool, skip *sesConn) {
	l.mu.RLock()
	fn := l.onChange
	l.mu.RUnlock()
	if fn != nil {
		fn(user, rows, adopted, skip)
	}
}

func (s *Sessions) sesWelcomeLogs(c *sesConn) []proto.Devlog {
	if c.joining.pending() {
		return nil
	}
	var frames []proto.Devlog
	for _, user := range slices.Sorted(maps.Keys(s.sesLogUsers(c))) {
		frames = append(frames, s.cfg.Logs.frames(user)...)
	}
	return frames
}

func (s *Sessions) sesLogUsers(c *sesConn) map[string]bool {
	users := map[string]bool{}
	if c.home != nil && s.cfg.Logs.Known(c.device) {
		users = s.cfg.Home.LogUsers(c.home)
	}
	if c.home == nil || c.paired || s.cfg.Logs.IsOwner(c.device) {
		users[s.cfg.Logs.Owner()] = true
	}
	return users
}

func (s *Sessions) sesDevlog(c *sesConn, data json.RawMessage) {
	var dl proto.Devlog
	if proto.Decode(data, &dl) != nil {
		return
	}
	if c.joining.pending() {
		c.joining.stage(dl)
		return
	}
	if s.cfg.Logs.Owner() == "" {
		s.sesStageAdoption(c, dl)
		return
	}
	if err := s.cfg.Logs.dvlReceive(dl, c); err != nil {
		s.cfg.Report(err)
	}
}

func (s *Sessions) sesStageAdoption(c *sesConn, dl proto.Devlog) {
	if !c.paired {
		return
	}
	if c.staging[dl.User] == nil {
		c.staging[dl.User] = &identity.Log{User: dl.User}
	}
	for _, env := range dl.Entries {
		if _, err := c.staging[dl.User].Apply(env); err != nil {
			delete(c.staging, dl.User)
			return
		}
	}
	s.sesFinishAdoption(c, dl, c.staging[dl.User])
}

func (s *Sessions) sesFinishAdoption(c *sesConn, dl proto.Devlog, cand *identity.Log) {
	if _, err := s.cfg.Logs.dvlAdopt(c.device, cand, c); err != nil {
		delete(c.staging, dl.User)
		s.cfg.Report(err)
		return
	}
	namesMe := func(e proto.Envelope) bool { return dvlRowOf(e).Device == s.cfg.ID }
	if slices.ContainsFunc(dl.Entries, namesMe) {
		delete(c.staging, dl.User)
	}
}

func (s *Sessions) sesLogChanged(user string, rows []proto.DeviceRow, adopted bool, skip *sesConn) {
	if adopted {
		s.sesAdopted(user, skip)
	}
	s.cfg.Home.Event("device", rows)
	envs := make([]proto.Envelope, len(rows))
	for i, r := range rows {
		envs[i] = r.Env
	}
	frames := dvlBatchFrames(user, envs)
	memberLog := user != s.cfg.Logs.Owner()
	for _, c := range s.sesConns() {
		if memberLog && dvlRevoked(c.device, rows) {
			c.Close()
			continue
		}
		s.sesSyncOne(c, user, frames, skip)
	}
}

func dvlRevoked(device string, rows []proto.DeviceRow) bool {
	return slices.ContainsFunc(rows, func(r proto.DeviceRow) bool {
		return r.Op == "revoke" && r.Device == device
	})
}

func (s *Sessions) sesAdopted(user string, skip *sesConn) {
	if s.cfg.OnAdopt != nil {
		s.cfg.OnAdopt(user)
	}
	if s.onPairAdopt != nil {
		s.onPairAdopt(skip)
	}
}

func (s *Sessions) sesSyncOne(c *sesConn, user string, frames []proto.Devlog, skip *sesConn) {
	if s.sesShouldForward(c, user, skip) {
		for _, dl := range frames {
			if c.send(dl) != nil {
				return
			}
		}
	}
}

func (s *Sessions) sesShouldForward(c *sesConn, user string, skip *sesConn) bool {
	s.mu.Lock()
	ok := c != skip && c.welcomed
	s.mu.Unlock()
	return ok && !c.joining.pending() && s.sesLogUsers(c)[user]
}

func dvlRowOf(env proto.Envelope) proto.DeviceRow {
	var ch proto.DeviceChange
	json.Unmarshal([]byte(env.Payload), &ch)
	if ch.Op != "add" {
		ch.Kind, ch.Name = "", ""
	}
	return proto.DeviceRow{RowHeader: proto.RowHeader{Type: "device", ID: fmt.Sprintf("%s:%d", ch.User, ch.Version)}, DeviceChange: ch, Env: env}
}

func dvlBatchFrames(user string, entries []proto.Envelope) []proto.Devlog {
	room := ctlLimit - len(dvlMustEncode(proto.Devlog{Type: "devlog", User: user, Entries: []proto.Envelope{}}))
	var frames []proto.Devlog
	var batch []proto.Envelope
	left := room
	for _, env := range entries {
		raw := dvlMustEncode(env)
		if len(raw) > left {
			frames, batch, left = append(frames, proto.Devlog{Type: "devlog", User: user, Entries: batch}), nil, room
		}
		batch = append(batch, env)
		left -= len(raw) + 1
	}
	return append(frames, proto.Devlog{Type: "devlog", User: user, Entries: batch})
}

func dvlMustEncode(v any) []byte { raw, _ := encodeCtl(v); return raw }
