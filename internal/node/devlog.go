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

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const dvlCtlLimit = 61_440

func init() { home.Events.Add("device", dvlHomeEvent) }

// Logs holds the verified device logs of a node's owner and group members.
// Lock order: writeMu -> Store.Write / mu; mu is a leaf lock guarding owner,
// logs, kept and onChange, never held across Store.Write, home.Loop or network writes.
type Logs struct {
	db          *store.Store
	self, owner string
	writeMu     sync.Mutex
	mu          sync.RWMutex
	logs        map[string]*identity.Log
	kept        map[string]bool
	onChange    func(user string, rows []proto.DeviceRow, adopted bool, skip *sesConn)
}

// OpenLogs loads the owner setting and verified device logs from db.
func OpenLogs(db *store.Store, selfID string) (*Logs, error) {
	l := &Logs{db: db, self: selfID, logs: map[string]*identity.Log{}, kept: map[string]bool{}}
	load := func(tx *store.Tx) error { return l.dvlLoad(tx) }
	if err := db.Write(load); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Logs) dvlLoad(tx *store.Tx) error {
	var raw string
	if err := tx.QueryRow("SELECT value FROM setting WHERE key='owner'").Scan(&raw); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &l.owner); err != nil {
			return err
		}
		l.kept[l.owner] = true
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
		var user, raw string
		var env proto.Envelope
		if rows.Scan(&user, &raw); proto.Decode([]byte(raw), &env) != nil {
			return identity.BadRequest
		}
		if l.logs[user] == nil {
			l.logs[user] = &identity.Log{User: user}
		}
		if _, err := l.logs[user].Apply(env); err != nil {
			return err
		}
		l.kept[user] = true
	}
	return rows.Err()
}

// Owner returns the user id of this node's owner, or empty before adoption.
func (l *Logs) Owner() string { l.mu.RLock(); defer l.mu.RUnlock(); return l.owner }

// IsOwner reports whether device is a currently valid device of this node's owner.
func (l *Logs) IsOwner(device string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.owner != "" && l.logs[l.owner] != nil && l.logs[l.owner].Valid(device)
}

// Known reports whether device is valid in any device log kept by this node.
func (l *Logs) Known(device string) bool { return l.UserOf(device) != "" }

// UserOf returns the person whose log holds device as valid, or empty.
func (l *Logs) UserOf(device string) string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, u := range slices.Sorted(maps.Keys(l.logs)) {
		if l.logs[u].Valid(device) {
			return u
		}
	}
	return ""
}

// Log returns an independent copy of user's verified log, or nil when none is held.
func (l *Logs) Log(user string) *identity.Log {
	l.mu.RLock()
	cur := l.logs[user]
	if cur == nil {
		l.mu.RUnlock()
		return nil
	}
	entries := slices.Clone(cur.Entries)
	l.mu.RUnlock()
	copied, _ := identity.Replay(user, entries)
	return copied
}

// Keep marks user as a person whose device log this node stores and forwards.
func (l *Logs) Keep(user string) {
	if identity.IsDevice(user) {
		l.mu.Lock()
		l.kept[user] = true
		l.mu.Unlock()
	}
}

// Frames returns user's full device log from version 1 as ctl devlog frames.
func (l *Logs) Frames(user string) []proto.Devlog {
	if log := l.Log(user); log != nil {
		return dvlBatchFrames(user, log.Entries)
	}
	return nil
}

// Append applies one entry to its user's kept log, persists it and notifies peers.
func (l *Logs) Append(env proto.Envelope) (proto.DeviceRow, error) {
	row := dvlRowOf(env)
	if !identity.IsDevice(row.User) {
		return proto.DeviceRow{}, identity.BadRequest
	}
	rows, err := l.dvlApplyKept(row.User, []proto.Envelope{env}, true)
	if len(rows) > 0 {
		l.dvlNotify(row.User, rows, false, nil)
	}
	return row, err
}

// Receive applies a devlog push received from a peer.
func (l *Logs) Receive(dl proto.Devlog) error { return l.dvlReceive(dl, nil) }

func (l *Logs) dvlReceive(dl proto.Devlog, skip *sesConn) error {
	rows, err := l.dvlApplyKept(dl.User, dl.Entries, false)
	if len(rows) > 0 {
		l.dvlNotify(dl.User, rows, false, skip)
	}
	return err
}
func (l *Logs) dvlApplyKept(user string, entries []proto.Envelope, strict bool) ([]proto.DeviceRow, error) {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()
	l.mu.RLock()
	kept := l.kept[user]
	l.mu.RUnlock()
	if !kept {
		if strict {
			return nil, identity.UnknownDevice
		}
		return nil, nil
	}
	return l.dvlCommitEntries(user, cmp.Or(l.Log(user), &identity.Log{User: user}), entries, strict)
}
func (l *Logs) dvlCommitEntries(user string, log *identity.Log, entries []proto.Envelope, strict bool) ([]proto.DeviceRow, error) {
	rows, err := dvlCollectRows(log, entries, strict)
	if len(rows) == 0 {
		return nil, err
	}
	write := func(tx *store.Tx) error { return dvlPutEntries(tx, user, rows) }
	if err := l.db.Write(write); err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.kept[user], l.logs[user] = true, log
	l.mu.Unlock()
	return rows, nil
}
func dvlCollectRows(log *identity.Log, entries []proto.Envelope, strict bool) ([]proto.DeviceRow, error) {
	var rows []proto.DeviceRow
	for _, env := range entries {
		added, err := log.Apply(env)
		if err != nil && strict {
			return nil, err
		}
		if added {
			rows = append(rows, dvlRowOf(env))
		}
	}
	return rows, nil
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
	l.owner, l.kept[cand.User], l.logs[cand.User] = cand.User, true, cand
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

func (s *Sessions) sesWelcomeLogs(c *sesConn, groups []proto.GroupSummary) []proto.Devlog {
	s.cfg.Logs.mu.RLock()
	users := slices.Sorted(maps.Keys(s.cfg.Logs.logs))
	s.cfg.Logs.mu.RUnlock()
	var out []proto.Devlog
	for _, u := range users {
		if s.sesPeerKeeps(c, u, len(groups) > 0) {
			out = append(out, s.cfg.Logs.Frames(u)...)
		}
	}
	return out
}
func (s *Sessions) sesPeerKeeps(c *sesConn, user string, hasGroups bool) bool {
	return hasGroups || s.cfg.Logs.UserOf(c.device) == user || (c.paired && user == s.cfg.Logs.Owner())
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
	if s.cfg.Home != nil {
		s.cfg.Home.Event("device", rows)
	}
	s.mu.Lock()
	conns := slices.Collect(maps.Keys(s.conns))
	s.mu.Unlock()
	envs := make([]proto.Envelope, len(rows))
	for i, r := range rows {
		envs[i] = r.Env
	}
	frames := dvlBatchFrames(user, envs)
	for _, c := range conns {
		s.sesSyncOne(c, user, rows, frames, skip)
	}
}
func (s *Sessions) sesSyncOne(c *sesConn, user string, rows []proto.DeviceRow, frames []proto.Devlog, skip *sesConn) {
	isRevoked := func(r proto.DeviceRow) bool { return r.Op == "revoke" && r.Device == c.device }
	if slices.ContainsFunc(rows, isRevoked) {
		c.raw.Close()
		return
	}
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
	return ok && s.sesPeerKeeps(c, user, c.home != nil && len(s.cfg.Home.Groups(c.home)) > 0)
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

func dvlHomeEvent(s *home.State, _ home.Disk, _ int64, event any) home.Result {
	rows, ok := event.([]proto.DeviceRow)
	if !ok || len(rows) == 0 || !dvlConcerns(s, rows[0].User) {
		return home.Result{}
	}
	held := 0
	if log := s.Logs[rows[0].User]; log != nil {
		held = len(log.Entries)
	}
	return dvlBuildHomeResult(s, rows, held)
}
func dvlBuildHomeResult(s *home.State, rows []proto.DeviceRow, held int) home.Result {
	var res home.Result
	for _, r := range rows {
		if r.Version > held {
			if s.Online[r.Device] && (r.Op == "add") != s.Serves(r.Device) {
				res.GroupsChanged = true
			}
			res.Rows = append(res.Rows, home.RowOf(r))
		}
	}
	return res
}
func dvlConcerns(s *home.State, user string) bool {
	if user == s.Owner {
		return true
	}
	for _, m := range s.People {
		if m.User == user {
			return true
		}
	}
	return false
}
