package host

import (
	"context"
	"encoding/json"
	"io"
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
	}
	return p
}

func watchItem(h *Host, v verifiedItem) {
	slot := aiSlot{group: v.group, member: string(v.item.AI)}
	if state, _ := h.MemberState(slot.group, slot.member); state == "takeover" {
		var card proto.MsgRow
		if err := proto.Decode(v.wire.Msg, &card); err != nil {
			panic(err)
		}
		member, _ := lookupRow[proto.MemberRow](h, slot.group, "member", card.By)
		p := watchState(h)
		p.mu.Lock()
		if p.grants[slot] == nil {
			p.grants[slot] = map[string]string{}
		}
		p.grants[slot][member.User] = card.By
		p.mu.Unlock()
	}
	ai, _, _ := lookupLocalAI(h, v.group, slot.member)
	row := AuditRow{Group: v.group, Agent: ai.agent, What: AuditDelivered, Item: v.item.Item, Home: v.home, Detail: v.item.Why, By: v.home}
	if recordAudit(h, row) == nil {
		sendAck(h, v.home, v.group, v.item.Item)
	}
}

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
		go p.monitor(from)
	}
	return nil, nil
}

func watchDeviceUser(h *Host, group, device string) string {
	var latest proto.DeviceRow
	for _, raw := range h.Rows.Of(group, "device") {
		var row proto.DeviceRow
		if err := proto.Decode(raw, &row); err != nil {
			panic(err)
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

func (h *Host) ServeWatch(t Terminal) {
	g, m, valid := watchLabel(t.Label())
	if !valid {
		ptyRejectTerminal(t)
		return
	}
	key := watchKey{aiSlot{g, m}, t.Device()}
	p := watchState(h)
	w, ok := p.attach(key, t)
	if !ok {
		ptyRejectTerminal(t)
		return
	}
	if size, known := ptyState(h).sizeOf(key.aiSlot); known {
		p.resize(key.aiSlot, size.cols, size.rows)
	}
	go p.writeLoop(w)
	var b [1]byte
	t.Read(b[:])
	p.mu.Lock()
	if p.open[key] == w {
		p.closeKey(key)
	}
	p.mu.Unlock()
	ptyCopy(io.Discard, t)
}

func watchLabel(label string) (string, string, bool) {
	rest, ok := strings.CutPrefix(label, "watch:")
	if !ok {
		return "", "", false
	}
	return ptyParseLabel("pty:" + rest)
}

func (p *watchPart) attach(key watchKey, t Terminal) (*watchWriter, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.ok[key]; !ok {
		return nil, false
	}
	if p.open[key] != nil {
		return nil, false
	}
	w := &watchWriter{term: t, wake: make(chan struct{}, 1)}
	p.open[key] = w
	return w, true
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
		link.Send(proto.PTYResize{Type: "pty.resize", Group: proto.ID(slot.group), AI: proto.ID(slot.member), Cols: cols, Rows: rows})
	}
}

func (p *watchPart) stop(slot aiSlot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key := range p.ok {
		if key.aiSlot == slot {
			p.closeKey(key)
			delete(p.ok, key)
		}
	}
	delete(p.grants, slot)
}

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
	defer p.mu.Unlock()
	if !p.revoke(aiSlot{string(req.Group), string(req.Member)}, req.User) {
		return nil, &Error{"not_found", "watch grant not found"}
	}
	return nil, nil
}

func watchGone(h *Host, group, member string) {
	p := watchState(h)
	p.mu.Lock()
	defer p.mu.Unlock()
	for slot, grants := range p.grants {
		if slot.group == group {
			p.revokeMember(slot, grants, member)
		}
	}
}

func (p *watchPart) revokeMember(slot aiSlot, grants map[string]string, member string) {
	for user, by := range grants {
		if by == member {
			p.revoke(slot, user)
		}
	}
}
