package host

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() { Register(Surface, "watch.end", watchEnd) }

type watchKey struct {
	aiSlot
	device string
}

type watchOK struct {
	from Owner
	user string
}

type watchWriter struct {
	term   Terminal
	queue  [][]byte
	bytes  int
	wake   chan struct{}
	closed bool
}

type watchPart struct {
	mu          sync.Mutex
	grants      map[aiSlot]map[string]string
	ok          map[watchKey]watchOK
	open        map[watchKey]*watchWriter
	connections map[Owner]bool
	ended       func(aiSlot, []string)
}

func watchState(h *Host) *watchPart {
	p := Part[watchPart](h)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.grants == nil {
		p.grants = map[aiSlot]map[string]string{}
		p.ok = map[watchKey]watchOK{}
		p.open = map[watchKey]*watchWriter{}
		p.connections = map[Owner]bool{}
		p.ended = h.watchEnded
	}
	return p
}

func watchItem(h *Host, v verifiedItem) {
	slot := aiSlot{group: v.group, member: string(v.item.AI)}
	var card proto.MsgRow
	err := proto.Decode(v.wire.Msg, &card)
	member, _ := lookupRow[proto.MemberRow](h, slot.group, "member", card.By)
	if state, _ := h.MemberState(slot.group, slot.member); state == "takeover" && err == nil && member.User != "" {
		p := watchState(h)
		p.mu.Lock()
		if p.grants[slot] == nil {
			p.grants[slot] = map[string]string{}
		}
		p.grants[slot][member.User] = card.By
		p.mu.Unlock()
		h.watchersChanged()
	}
	ai, _, _ := lookupLocalAI(h, v.group, slot.member)
	row := AuditRow{Group: v.group, Agent: ai.agent, What: AuditDelivered, Item: v.item.Item, Home: v.home, Detail: v.item.Why, By: v.home}
	if recordAudit(h, row) == nil {
		sendAck(h, v.home, v.group, v.item.Item)
	}
}

// WatchStart authorizes a device's direct connection for a granted terminal.
func (h *Host) WatchStart(from Owner, device string, data json.RawMessage) (any, error) {
	var req proto.WatchStart
	if err := proto.Decode(data, &req); err != nil {
		return nil, &Error{"bad_request", "invalid watch.start frame"}
	}
	slot := aiSlot{group: string(req.Group), member: string(req.Member)}
	user := watchDeviceUser(h, slot.group, device)
	p := watchState(h)
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, granted := p.grants[slot][user]; !granted {
		return nil, &Error{"forbidden", "watch grant required"}
	}
	if from.Path() != "direct" {
		return nil, &Error{"relay", "watch requires a direct path"}
	}
	p.ok[watchKey{slot, device}] = watchOK{from, user}
	if !p.connections[from] {
		p.connections[from] = true
		h.bg(func(f func()) { go f() }, "watch_conn", func() { p.monitor(from) })
	}
	return nil, nil
}

func watchDeviceUser(h *Host, group, device string) string {
	var latest proto.DeviceRow
	for _, raw := range h.Rows.Of(group, "device") {
		var row proto.DeviceRow
		if proto.Decode(raw, &row) != nil {
			continue
		}
		if row.Device == device && row.Version > latest.Version {
			latest = row
		}
	}
	if latest.Op == "add" {
		return latest.User
	}
	return ""
}

func (p *watchPart) monitor(from Owner) {
	for from.Path() == "direct" {
		select {
		case <-from.Done():
			p.disconnect(from)
			return
		case <-from.PathChanged():
		}
	}
	p.disconnect(from)
}

func (p *watchPart) disconnect(from Owner) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, ok := range p.ok {
		if ok.from == from {
			p.closeKey(key)
			delete(p.ok, key)
		}
	}
	delete(p.connections, from)
}

func (p *watchPart) closeKey(key watchKey) {
	if w := p.open[key]; w != nil {
		w.closed = true
		w.queue = nil
		w.term.Close()
		select {
		case w.wake <- struct{}{}:
		default:
		}
		delete(p.open, key)
	}
}

// ServeWatch serves a receive-only terminal channel authorized by WatchStart.
func (h *Host) ServeWatch(t Terminal) {
	g, m, valid := watchLabel(t.Label())
	if !valid {
		rejectWatch(t)
		return
	}
	key := watchKey{aiSlot{g, m}, t.Device()}
	p := watchState(h)
	w, from := p.attach(key, t)
	if from == nil {
		rejectWatch(t)
		return
	}
	sendWatchSize(h, key.aiSlot, from)
	h.bg(func(f func()) { go f() }, "watch_write", func() { p.writeLoop(w) }, "g", g)
	var b [1]byte
	t.Read(b[:])
	p.mu.Lock()
	if p.open[key] == w {
		p.closeKey(key)
	}
	p.mu.Unlock()
	io.Copy(io.Discard, t)
}

func rejectWatch(t Terminal) {
	t.Close()
	io.Copy(io.Discard, t)
}

func sendWatchSize(h *Host, slot aiSlot, link Owner) {
	if p := ptyState(h).lookup(slot); p != nil {
		p.mu.Lock()
		size := p.watchSize
		p.mu.Unlock()
		if size.Type != "" {
			link.Send(size)
		}
	}
}

func watchLabel(label string) (string, string, bool) {
	rest, ok := strings.CutPrefix(label, "watch:")
	if !ok {
		return "", "", false
	}
	return ptyParseLabel("pty:" + rest)
}

func (p *watchPart) attach(key watchKey, t Terminal) (*watchWriter, Owner) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.ok[key]; !ok {
		return nil, nil
	}
	if p.open[key] != nil {
		return nil, nil
	}
	w := &watchWriter{term: t, wake: make(chan struct{}, 1)}
	p.open[key] = w
	return w, p.ok[key].from
}

func (p *watchPart) next(w *watchWriter) ([]byte, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if w.closed {
		return nil, false
	}
	if len(w.queue) == 0 {
		return nil, true
	}
	b := w.queue[0]
	w.queue = w.queue[1:]
	return b, true
}

func (p *watchPart) writeLoop(w *watchWriter) {
	for {
		b, open := p.next(w)
		if !open {
			return
		}
		if b == nil {
			<-w.wake
			continue
		}
		_, err := w.term.Write(b)
		p.mu.Lock()
		w.bytes -= len(b)
		p.mu.Unlock()
		if err != nil {
			w.term.Close()
			return
		}
	}
}

type watchFanout struct {
	io.Writer
	part *watchPart
	slot aiSlot
}

func (w watchFanout) Write(b []byte) (int, error) {
	n, err := w.Writer.Write(b)
	w.part.fanout(w.slot, b[:n])
	return n, err
}

func (p *watchPart) fanout(slot aiSlot, b []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, w := range p.open {
		if key.aiSlot != slot {
			continue
		}
		if w.bytes+len(b) > 1<<20 {
			p.closeKey(key)
			continue
		}
		w.queue = append(w.queue, slices.Clone(b))
		w.bytes += len(b)
		select {
		case w.wake <- struct{}{}:
		default:
		}
	}
}

func (p *watchPart) resize(slot aiSlot, cols, rows int) {
	p.mu.Lock()
	links := []Owner{}
	for key := range p.open {
		if key.aiSlot == slot {
			links = append(links, p.ok[key].from)
		}
	}
	p.mu.Unlock()
	for _, link := range links {
		watchResize(link, slot, cols, rows)
	}
}

func watchResize(link Owner, slot aiSlot, cols, rows int) {
	link.Send(proto.PTYResize{Type: "pty.resize", Group: proto.ID(slot.group), AI: proto.ID(slot.member), Cols: cols, Rows: rows})
}

func (p *watchPart) stop(slot aiSlot) {
	p.mu.Lock()
	users := slices.Collect(maps.Keys(p.grants[slot]))
	for key := range p.ok {
		if key.aiSlot == slot {
			p.closeKey(key)
			delete(p.ok, key)
		}
	}
	delete(p.grants, slot)
	p.mu.Unlock()
	p.ended(slot, users)
}

// Watchers returns the sorted user IDs holding a grant for this terminal.
func (h *Host) Watchers(group, member string) []string {
	p := watchState(h)
	p.mu.Lock()
	defer p.mu.Unlock()
	out := []string{}
	for user := range p.grants[aiSlot{group, member}] {
		out = append(out, user)
	}
	slices.Sort(out)
	return out
}

func (p *watchPart) revoke(slot aiSlot, user string) bool {
	if _, ok := p.grants[slot][user]; !ok {
		return false
	}
	delete(p.grants[slot], user)
	for key, ok := range p.ok {
		if key.aiSlot == slot && ok.user == user {
			p.closeKey(key)
			delete(p.ok, key)
		}
	}
	return true
}

func watchEnd(_ context.Context, h *Host, _ Owner, _ string, data json.RawMessage) (any, error) {
	var req proto.WatchEnd
	if err := proto.Decode(data, &req); err != nil {
		return nil, &Error{"bad_request", "invalid watch.end frame"}
	}
	p := watchState(h)
	p.mu.Lock()
	slot := aiSlot{string(req.Group), string(req.Member)}
	ok := p.revoke(slot, req.User)
	p.mu.Unlock()
	if !ok {
		return nil, &Error{"not_found", "watch grant not found"}
	}
	p.ended(slot, []string{req.User})
	return nil, nil
}

func watchGone(h *Host, group, member string) {
	p := watchState(h)
	p.mu.Lock()
	ended := map[aiSlot][]string{}
	for slot, grants := range p.grants {
		if slot.group == group {
			ended[slot] = p.revokeMember(slot, grants, member)
		}
	}
	p.mu.Unlock()
	for slot, users := range ended {
		p.ended(slot, users)
	}
}

func (p *watchPart) revokeMember(slot aiSlot, grants map[string]string, member string) []string {
	users := []string{}
	for user, by := range grants {
		if by == member {
			p.revoke(slot, user)
			users = append(users, user)
		}
	}
	return users
}

// Watching reports whether the device's person holds any watch grant here.
func (h *Host) Watching(device string) bool {
	p := watchState(h)
	p.mu.Lock()
	defer p.mu.Unlock()
	for slot, grants := range p.grants {
		if _, ok := grants[watchDeviceUser(h, slot.group, device)]; ok {
			return true
		}
	}
	return false
}

func (p *watchPart) hasUser(user string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, grants := range p.grants {
		if _, ok := grants[user]; ok {
			return true
		}
	}
	return false
}

func (h *Host) watchEnded(slot aiSlot, users []string) {
	for _, user := range users {
		if !watchState(h).hasUser(user) && h.WatchEnded != nil {
			h.WatchEnded(slot.group, user)
		}
	}
	if len(users) > 0 {
		h.watchersChanged()
	}
}

func (h *Host) watchersChanged() {
	if h.WatchersChanged != nil {
		h.WatchersChanged()
	}
}
