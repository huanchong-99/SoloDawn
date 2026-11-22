package host

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const joinAttachTimeout = 15 * time.Second

func init() {
	Register(Surface, "host.join", handleHostJoin)
	Register(Surface, "host.leave", handleHostLeave)
	Register(Attached, "join", joinAttached)
}

type joinStart struct {
	device string
	ctx    context.Context
}

type joinConfig struct {
	account    string
	projectDir string
	approvers  []string
}

const joinConfigQuerySQL = `SELECT
	coalesce((SELECT account FROM agent WHERE agent = ?), ''),
	coalesce((SELECT value FROM setting WHERE key = 'approvers'), '[]')`

const joinHomesSettingSQL = `SELECT coalesce((SELECT value FROM setting WHERE key = 'homes'), '[]')`

// joinPart holds per-host home connection tracking and Attach waiters.
//
// Lock order: syncMu serializes connection reconciliation across Store reads;
// mu is a strict leaf lock that guards only joinPart fields and never calls
// into Host, Sessions, Rows, Store, Connect or cancel funcs.
type joinPart struct {
	syncMu   sync.Mutex
	mu       sync.Mutex
	conns    map[string]context.CancelFunc
	inflight map[string]int
	waiters  map[string][]chan struct{}
}

func joinStateOf(h *Host) *joinPart {
	jp := Part[joinPart](h)
	jp.mu.Lock()
	if jp.conns == nil {
		jp.conns = map[string]context.CancelFunc{}
		jp.inflight = map[string]int{}
		jp.waiters = make(map[string][]chan struct{})
	}
	jp.mu.Unlock()
	return jp
}

// EnsureHomeConnections opens one persistent connection through h.Connect for
// each distinct remembered home.
func (h *Host) EnsureHomeConnections() error {
	return joinSyncNow(h, joinStateOf(h))
}

func (jp *joinPart) pinHome(device string) {
	jp.mu.Lock()
	jp.inflight[device]++
	jp.mu.Unlock()
}

func (jp *joinPart) unpinHome(device string) {
	jp.mu.Lock()
	jp.inflight[device]--
	if jp.inflight[device] == 0 {
		delete(jp.inflight, device)
	}
	jp.mu.Unlock()
}

func (jp *joinPart) addWaiter(device string) chan struct{} {
	// Single reader: the joinWaitChannel call in joinAwaitAttach that created ch.
	ch := make(chan struct{})
	jp.mu.Lock()
	jp.waiters[device] = append(jp.waiters[device], ch)
	jp.mu.Unlock()
	return ch
}

func (jp *joinPart) removeWaiter(device string, ch chan struct{}) {
	match := func(cur chan struct{}) bool { return cur == ch }
	jp.mu.Lock()
	list := slices.DeleteFunc(jp.waiters[device], match)
	if len(list) == 0 {
		delete(jp.waiters, device)
	} else {
		jp.waiters[device] = list
	}
	jp.mu.Unlock()
}

func joinAttached(h *Host, device string) {
	jp := joinStateOf(h)
	jp.mu.Lock()
	list := jp.waiters[device]
	delete(jp.waiters, device)
	jp.mu.Unlock()
	for _, ch := range list {
		close(ch)
	}
}

func joinSyncNow(h *Host, jp *joinPart) error {
	jp.syncMu.Lock()
	defer jp.syncMu.Unlock()
	homes, err := joinSavedHomes(h.Store)
	if err != nil {
		return err
	}
	needed := joinNeededHomes(homes)
	jp.mu.Lock()
	for dev := range jp.inflight {
		needed[dev] = true
	}
	toStop := joinPlanStopsLocked(jp, needed)
	toStart := joinPlanStartsLocked(jp, h.Connect != nil, needed)
	jp.mu.Unlock()
	joinApplySync(h, toStop, toStart)
	return nil
}

func joinApplySync(h *Host, toStop []context.CancelFunc, toStart []joinStart) {
	for _, stop := range toStop {
		stop()
	}
	for _, start := range toStart {
		h.Connect(start.ctx, start.device)
	}
}

func joinPlanStopsLocked(jp *joinPart, needed map[string]bool) []context.CancelFunc {
	var stops []context.CancelFunc
	for dev, stop := range jp.conns {
		if !needed[dev] {
			stops = append(stops, stop)
			delete(jp.conns, dev)
		}
	}
	return stops
}

func joinPlanStartsLocked(jp *joinPart, canConnect bool, needed map[string]bool) []joinStart {
	if !canConnect {
		return nil
	}
	var starts []joinStart
	for _, dev := range slices.Sorted(maps.Keys(needed)) {
		if _, open := jp.conns[dev]; !open {
			connCtx, cancel := context.WithCancel(context.Background())
			jp.conns[dev] = cancel
			starts = append(starts, joinStart{device: dev, ctx: connCtx})
		}
	}
	return starts
}

func joinNeededHomes(homes []proto.Home) map[string]bool {
	needed := map[string]bool{}
	for _, entry := range homes {
		needed[entry.Home] = true
	}
	return needed
}

func joinSavedHomes(db *store.Store) ([]proto.Home, error) {
	var raw string
	read := func(tx *store.Tx) error {
		return tx.QueryRow(joinHomesSettingSQL).Scan(&raw)
	}
	if err := db.Write(read); err != nil {
		return nil, err
	}
	var homes []proto.Home
	if err := json.Unmarshal([]byte(raw), &homes); err != nil {
		return nil, err
	}
	return homes, nil
}

func handleHostJoin(ctx context.Context, h *Host, _ Owner, by string, data json.RawMessage) (any, error) {
	p, err := joinParseJoin(data)
	if err != nil {
		return nil, err
	}
	cfg, err := joinPrepareJoin(h, p, string(p.Group))
	if err != nil {
		return nil, err
	}
	return nil, joinExecuteJoin(ctx, h, joinStateOf(h), p, cfg, by)
}

func joinParseJoin(data json.RawMessage) (proto.HostJoin, error) {
	var p proto.HostJoin
	if proto.Decode(data, &p) != nil || !joinValidJoin(p) {
		return p, &Error{Code: "bad_request", Message: "invalid host.join payload"}
	}
	return p, nil
}

func joinValidJoin(p proto.HostJoin) bool {
	return p.Home != "" && p.Name != "" && agtValidAgent(p.Agent)
}

func joinPrepareJoin(h *Host, p proto.HostJoin, group string) (joinConfig, error) {
	if _, _, exists := joinFindAgentMember(h, group, p.Agent); exists {
		return joinConfig{}, &Error{Code: "conflict", Message: "that agent is already in the group"}
	}
	return joinLoadConfig(h, p.Agent, group)
}

func joinLoadConfig(h *Host, agent, group string) (joinConfig, error) {
	var account, listJSON string
	read := func(tx *store.Tx) error {
		return tx.QueryRow(joinConfigQuerySQL, agent).Scan(&account, &listJSON)
	}
	if err := h.Store.Write(read); err != nil {
		return joinConfig{}, err
	}
	if account == "" {
		return joinConfig{}, &Error{Code: "conflict", Message: "declare the agent's account with agent.set before joining a group"}
	}
	approvers := []string{}
	if err := json.Unmarshal([]byte(listJSON), &approvers); err != nil {
		return joinConfig{}, err
	}
	return joinConfig{
		account:    account,
		projectDir: projectDir(h, group),
		approvers:  approvers,
	}, nil
}

func joinExecuteJoin(ctx context.Context, h *Host, jp *joinPart, p proto.HostJoin, cfg joinConfig, by string) error {
	jp.pinHome(p.Home)
	defer joinFinishPin(h, jp, p.Home)
	homeConn, err := joinConnectAndWait(ctx, h, jp, p.Home)
	if err != nil {
		return err
	}
	grp, err := joinFetchGroup(ctx, h, homeConn, p.Home, string(p.Group))
	if err != nil {
		return err
	}
	if grp.Kind == "team" {
		return &Error{Code: "conflict", Message: "a team group has no AI members"}
	}
	if err := joinPrepareWorkspace(ctx, cfg.projectDir, grp); err != nil {
		return err
	}
	memberID, err := joinSendAddAI(ctx, h, p, cfg)
	if err != nil {
		return err
	}
	return joinCommitJoined(h, p, string(memberID), grp.Name, by)
}

func joinFinishPin(h *Host, jp *joinPart, device string) {
	jp.unpinHome(device)
	joinSyncNow(h, jp)
}

func joinConnectAndWait(ctx context.Context, h *Host, jp *joinPart, device string) (Home, error) {
	if err := joinSyncNow(h, jp); err != nil {
		return nil, err
	}
	return joinWaitHome(ctx, h, jp, device)
}

func joinWaitHome(ctx context.Context, h *Host, jp *joinPart, device string) (Home, error) {
	if conn, ok := h.Home(device); ok {
		return conn, nil
	}
	if h.Connect == nil {
		return nil, errHomeUnreachable
	}
	waitCtx, cancel := context.WithTimeout(ctx, joinAttachTimeout)
	defer cancel()
	return joinAwaitAttach(waitCtx, h, jp, device)
}

func joinAwaitAttach(ctx context.Context, h *Host, jp *joinPart, device string) (Home, error) {
	for {
		waitCh := jp.addWaiter(device)
		if conn, ok := h.Home(device); ok {
			jp.removeWaiter(device, waitCh)
			return conn, nil
		}
		if !joinWaitChannel(ctx, waitCh) {
			jp.removeWaiter(device, waitCh)
			return nil, errHomeUnreachable
		}
	}
}

// joinWaitChannel is the single reader of ch, a channel created in addWaiter.
func joinWaitChannel(ctx context.Context, ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	case <-ctx.Done():
		return false
	}
}

func joinFetchGroup(ctx context.Context, h *Host, homeConn Home, device, group string) (proto.GroupRow, error) {
	if err := syncGroupState(ctx, h, homeConn, device, group); err != nil {
		return proto.GroupRow{}, err
	}
	if grp, ok := joinCachedGroup(h, group); ok {
		return grp, nil
	}
	return proto.GroupRow{}, &Error{Code: "not_found", Message: "the group does not exist"}
}

func joinCachedGroup(h *Host, group string) (proto.GroupRow, bool) {
	raw, ok := h.Rows.Get(group, "group", group)
	if !ok {
		return proto.GroupRow{}, false
	}
	var grp proto.GroupRow
	return grp, proto.Decode(raw, &grp) == nil
}

func joinPrepareWorkspace(ctx context.Context, projectDir string, grp proto.GroupRow) error {
	if _, err := EnsureGroupWorkspace(ctx, projectDir, grp.Repo, grp.Base); err != nil {
		return &Error{Code: "cli", Message: RedactSecrets(err.Error())}
	}
	return nil
}

func joinSendAddAI(ctx context.Context, h *Host, p proto.HostJoin, cfg joinConfig) (proto.ID, error) {
	memberID := proto.NewNonce()
	cmd := proto.MemberAddAI{
		MemberCommand: proto.MemberCommand{
			HomeCommand: homeHeader(h, string(p.Group), "", "member"),
			Op:          "add_ai",
			ID:          memberID,
		},
		CLI:       p.Agent,
		Name:      p.Name,
		Account:   cfg.account,
		Secretary: p.Secretary,
		Approvers: cfg.approvers,
	}
	_, err := h.Command(ctx, p.Home, marshalWire(cmd))
	return memberID, err
}

func joinCommitJoined(h *Host, p proto.HostJoin, member, groupName, by string) error {
	group := string(p.Group)
	run := func(tx *Tx) error {
		if err := joinDemoteIfSecretary(tx, h, group, p.Secretary); err != nil {
			return err
		}
		session := Session{
			SessionKey: SessionKey{Group: group, Member: member},
			Kind:       GroupSession,
			Folder:     "repo",
			Agent:      p.Agent,
			Home:       p.Home,
			Name:       p.Name,
			Secretary:  p.Secretary,
		}
		if err := tx.Create(session); err != nil {
			return err
		}
		if err := joinUpsertHomeTx(tx.Tx, proto.Home{Group: p.Group, Home: p.Home, Name: groupName}); err != nil {
			return err
		}
		_, err := h.Audit(tx.Tx, AuditRow{
			Group:  group,
			Agent:  p.Agent,
			What:   AuditSetting,
			Detail: "host.join " + p.Name,
			By:     by,
		})
		return err
	}
	return h.Sessions.Write(run)
}

func joinDemoteIfSecretary(tx *Tx, h *Host, group string, secretary bool) error {
	if !secretary {
		return nil
	}
	clearFlag := func(row *Session) { row.Secretary = false }
	for _, s := range h.Sessions.All() {
		if s.Group == group && s.Secretary {
			if err := tx.Update(s.SessionKey, clearFlag); err != nil {
				return err
			}
		}
	}
	return nil
}

func joinUpsertHomeTx(tx *store.Tx, entry proto.Home) error {
	homes, err := joinReadHomesTx(tx)
	if err != nil {
		return err
	}
	updated := joinPutHome(homes, entry)
	_, err = tx.Exec("INSERT OR REPLACE INTO setting(key, value) VALUES('homes', ?)", marshalWire(updated))
	return err
}

// RecordHome remembers a home and reconciles connections and state subscriptions.
func (h *Host) RecordHome(entry proto.Home, addrs []string) error {
	var moved bool
	write := func(tx *Tx) error {
		var err error
		moved, err = h.joinRecordHome(tx, entry, addrs)
		return err
	}
	if err := h.Sessions.Write(write); err != nil {
		return err
	}
	group := string(entry.Group)
	d := deliv(h)
	if d.replaceHome(group, entry.Home) || moved {
		h.Rows.Reset(group)
	}
	ensureGroupSynced(h, d, entry.Home, group)
	return h.EnsureHomeConnections()
}

func (h *Host) joinRecordHome(tx *Tx, entry proto.Home, addrs []string) (bool, error) {
	if err := joinUpsertHomeTx(tx.Tx, entry); err != nil {
		return false, err
	}
	if err := joinPutAddrs(tx.Tx, entry.Home, addrs, h.Now().UnixMilli()); err != nil {
		return false, err
	}
	return h.joinMoveSessions(tx, entry)
}

func joinPutAddrs(tx *store.Tx, device string, addrs []string, at int64) error {
	for _, addr := range addrs {
		if _, err := tx.Exec("INSERT OR REPLACE INTO peer(device, url, ok_at) VALUES(?, ?, ?)", device, addr, at); err != nil {
			return err
		}
	}
	return nil
}

func (h *Host) joinMoveSessions(tx *Tx, entry proto.Home) (bool, error) {
	moved := false
	setHome := func(row *Session) { row.Home = entry.Home }
	for _, session := range h.Sessions.All() {
		if session.Group != string(entry.Group) || session.Home == entry.Home {
			continue
		}
		if err := tx.Update(session.SessionKey, setHome); err != nil {
			return false, err
		}
		moved = true
	}
	return moved, nil
}

func joinPutHome(homes []proto.Home, entry proto.Home) []proto.Home {
	out := []proto.Home{}
	replaced := false
	for _, cur := range homes {
		if cur.Group == entry.Group {
			out = append(out, entry)
			replaced = true
		} else {
			out = append(out, cur)
		}
	}
	if !replaced {
		out = append(out, entry)
	}
	return out
}

func joinReadHomesTx(tx *store.Tx) ([]proto.Home, error) {
	var raw string
	if err := tx.QueryRow(joinHomesSettingSQL).Scan(&raw); err != nil {
		return nil, err
	}
	var homes []proto.Home
	if err := json.Unmarshal([]byte(raw), &homes); err != nil {
		return nil, err
	}
	return homes, nil
}

func handleHostLeave(ctx context.Context, h *Host, _ Owner, by string, data json.RawMessage) (any, error) {
	p, err := joinParseLeave(data)
	if err != nil {
		return nil, err
	}
	group := string(p.Group)
	member, home, ok := joinFindAgentMember(h, group, p.Agent)
	if !ok {
		return nil, &Error{Code: "not_found", Message: "that agent is not in the group"}
	}
	jp := joinStateOf(h)
	if err := joinRemoveFromHome(ctx, h, jp, p.Group, member, home); err != nil {
		return nil, err
	}
	closeMemberSessions(h, group, member)
	if err := joinCommitLeft(h, group, p.Agent, member, by); err != nil {
		return nil, err
	}
	joinSyncNow(h, jp)
	return nil, nil
}

func joinParseLeave(data json.RawMessage) (proto.HostLeave, error) {
	var p proto.HostLeave
	if proto.Decode(data, &p) != nil || !agtValidAgent(p.Agent) {
		return p, &Error{Code: "bad_request", Message: "invalid host.leave payload"}
	}
	return p, nil
}

func joinFindAgentMember(h *Host, group, agent string) (member, home string, ok bool) {
	for _, s := range h.Sessions.All() {
		if s.Group == group && s.Agent == agent {
			return s.Member, s.Home, true
		}
	}
	return "", "", false
}

func joinRemoveFromHome(ctx context.Context, h *Host, jp *joinPart, group proto.ID, member, home string) error {
	if _, err := joinConnectAndWait(ctx, h, jp, home); err != nil {
		return err
	}
	cmd := proto.MemberCommand{
		HomeCommand: homeHeader(h, string(group), "", "member"),
		Op:          "remove",
		ID:          proto.ID(member),
	}
	_, err := h.Command(ctx, home, marshalWire(cmd))
	return err
}

func joinCommitLeft(h *Host, group, agent, member, by string) error {
	run := func(tx *Tx) error {
		if err := joinEndMemberSessions(tx, h, group, member); err != nil {
			return err
		}
		if err := joinPruneHomeIfEmptyTx(tx.Tx, h, group); err != nil {
			return err
		}
		if agent == "" {
			return nil
		}
		_, err := h.Audit(tx.Tx, AuditRow{
			Group:  group,
			Agent:  agent,
			What:   AuditSetting,
			Detail: "host.leave",
			By:     by,
		})
		return err
	}
	return h.Sessions.Write(run)
}

func closeMemberSessions(h *Host, group, member string) []Session {
	sessions := memberSessions(h, group, member)
	for _, s := range sessions {
		Runner.Close(h, s.SessionKey)
	}
	return sessions
}

func leaveGoneMember(h *Host, group, member string) {
	if len(closeMemberSessions(h, group, member)) != 0 && joinCommitLeft(h, group, "", member, "") == nil {
		joinSyncNow(h, joinStateOf(h))
	}
}

func joinEndMemberSessions(tx *Tx, h *Host, group, member string) error {
	for _, s := range memberSessions(h, group, member) {
		if err := tx.End(s.SessionKey); err != nil {
			return err
		}
	}
	return nil
}

func joinPruneHomeIfEmptyTx(tx *store.Tx, h *Host, group string) error {
	if joinGroupHasSession(h, group) {
		return nil
	}
	homes, err := joinReadHomesTx(tx)
	if err != nil {
		return err
	}
	kept := []proto.Home{}
	for _, cur := range homes {
		if string(cur.Group) != group {
			kept = append(kept, cur)
		}
	}
	_, err = tx.Exec("INSERT OR REPLACE INTO setting(key, value) VALUES('homes', ?)", marshalWire(kept))
	return err
}

func joinGroupHasSession(h *Host, group string) bool {
	for _, s := range h.Sessions.All() {
		if s.Group == group {
			return true
		}
	}
	return false
}
