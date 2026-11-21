package node

import (
	"bytes"
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

const dvlCtlLimit = 61_440

// Logs holds the verified device log of this node's owner.
// Lock order: writeMu -> Store.Write / mu; mu is a leaf lock guarding owner,
// log and onChange, never held across Store.Write, home.Loop or network writes.
type Logs struct {
	db          *store.Store
	self, owner string
	writeMu     sync.Mutex
	mu          sync.RWMutex
	log         *identity.Log
	onChange    func(user string, rows []proto.DeviceRow, adopted bool, skip *sesConn)
}

// OpenLogs loads the owner setting and verified device log from db.
func OpenLogs(db *store.Store, selfID string) (*Logs, error) {
	l := &Logs{db: db, self: selfID}
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
	rows, err := tx.Query("SELECT env FROM device_log WHERE user = ? ORDER BY ver", l.owner)
	if err != nil {
		return err
	}
	defer rows.Close()
	log := &identity.Log{User: l.owner}
	for rows.Next() {
		var raw string
		var env proto.Envelope
		if rows.Scan(&raw); proto.Decode([]byte(raw), &env) != nil {
			return identity.BadRequest
		}
		if _, err := log.Apply(env); err != nil {
			return err
		}
	}
	if len(log.Entries) > 0 {
		l.log = log
	}
	return rows.Err()
}

// Owner returns the user id of this node's owner, or empty before adoption.
func (l *Logs) Owner() string { l.mu.RLock(); defer l.mu.RUnlock(); return l.owner }

// OwnerDevices returns the valid device ids of this node's owner, excluding self.
func (l *Logs) OwnerDevices() []string {
	out := []string{}
	log := l.Log(l.Owner())
	if log == nil {
		return out
	}
	for _, env := range log.Entries {
		dev := dvlRowOf(env).Device
		if dev != l.self && log.Valid(dev) {
			out = append(out, dev)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// UserOf returns this node's owner when device is valid in the owner's log, or empty.
func (l *Logs) UserOf(device string) string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.log != nil && l.log.Valid(device) {
		return l.owner
	}
	return ""
}

// IsOwner reports whether device is a currently valid device of this node's owner.
func (l *Logs) IsOwner(device string) bool { return l.UserOf(device) != "" }

// Known reports whether device is valid in the device log kept by this node.
func (l *Logs) Known(device string) bool { return l.UserOf(device) != "" }

// Log returns an independent copy of user's verified log, or nil when none is held.
func (l *Logs) Log(user string) *identity.Log {
	l.mu.RLock()
	if l.log == nil || user != l.owner {
		l.mu.RUnlock()
		return nil
	}
	entries := slices.Clone(l.log.Entries)
	l.mu.RUnlock()
	copied, _ := identity.Replay(user, entries)
	return copied
}

func (l *Logs) frames(user string) []proto.Devlog {
	if log := l.Log(user); log != nil {
		return dvlBatchFrames(user, log.Entries)
	}
	return nil
}

func (l *Logs) dvlReceive(dl proto.Devlog, skip *sesConn) error {
	rows, err := l.dvlCommitOwner(dl.User, dl.Entries)
	if len(rows) > 0 {
		l.dvlNotify(dl.User, rows, false, skip)
	}
	return err
}

func (l *Logs) dvlCommitOwner(user string, entries []proto.Envelope) ([]proto.DeviceRow, error) {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()
	if user != l.Owner() {
		return nil, nil
	}
	log := cmp.Or(l.Log(user), &identity.Log{User: user})
	rows := dvlCollectRows(log, entries)
	if len(rows) == 0 {
		return nil, nil
	}
	write := func(tx *store.Tx) error { return dvlPutEntries(tx, user, rows) }
	if err := l.db.Write(write); err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.log = log
	l.mu.Unlock()
	return rows, nil
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
	l.owner, l.log = cand.User, cand
	l.mu.Unlock()
	return nil
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
	if s.sesPeerKeeps(c) {
		return s.cfg.Logs.frames(s.cfg.Logs.Owner())
	}
	return nil
}

func (s *Sessions) sesPeerKeeps(c *sesConn) bool {
	return c.paired || s.cfg.Logs.IsOwner(c.device)
}

func (s *Sessions) sesDevlog(c *sesConn, data json.RawMessage) {
	var dl proto.Devlog
	if proto.Decode(data, &dl) != nil {
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
	if adopted && s.cfg.OnAdopt != nil {
		s.cfg.OnAdopt(user)
	}
	s.homeLoop().Event("device", rows)
	s.mu.Lock()
	conns := slices.Collect(maps.Keys(s.conns))
	s.mu.Unlock()
	envs := make([]proto.Envelope, len(rows))
	for i, r := range rows {
		envs[i] = r.Env
	}
	frames := dvlBatchFrames(user, envs)
	for _, c := range conns {
		s.sesSyncOne(c, frames, skip)
	}
}

func (s *Sessions) sesSyncOne(c *sesConn, frames []proto.Devlog, skip *sesConn) {
	if s.sesShouldForward(c, skip) {
		for _, dl := range frames {
			if c.send(dl) != nil {
				return
			}
		}
	}
}

func (s *Sessions) sesShouldForward(c *sesConn, skip *sesConn) bool {
	s.mu.Lock()
	ok := c != skip && c.welcomed
	s.mu.Unlock()
	return ok && s.sesPeerKeeps(c)
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
	room := dvlCtlLimit - len(dvlMustEncode(proto.Devlog{Type: "devlog", User: user, Entries: []proto.Envelope{}}))
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

func dvlEncode(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	err := enc.Encode(v)
	return bytes.TrimSuffix(b.Bytes(), []byte{'\n'}), err
}

func dvlMustEncode(v any) []byte { raw, _ := dvlEncode(v); return raw }
