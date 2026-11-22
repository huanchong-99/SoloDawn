package home

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Web Push subscriptions and encrypted short reminders for browsers that are
// offline when an event concerns their person (planning/contracts/commands.md
// section 1.7; planning/contracts/storage.md section 1, push_sub).
//
// Concurrency and lock order: wpushMu is a leaf lock protecting wpushHomes and
// each wpushHome.subs and wpushHome.loop. Nothing held under wpushMu calls
// Loop, Store, Disk or HTTP, so it never closes a cycle with Loop.mu or the
// store writer.

func init() {
	Commands.Add("push", wpushCmd)
	After.Add("push", wpushAfter)
}

const (
	wpushTitleLimit = 80
	wpushTTL        = 86400
	wpushWindow     = 5 * time.Second
	wpushSubscriber = "https://github.com/huanchong-99/SoloDawn"
)

var (
	wpushExtractors = map[string]func(*State, proto.MsgRow) wpushEvent{
		"approval": wpushApproval, "decision": wpushDecision, "brief": wpushBrief, "summary": wpushSummary,
	}
	wpushMu    sync.Mutex
	wpushHomes = map[string]*wpushHome{}
)

type wpushHome struct {
	subs map[string]wpushSub
	loop *Loop
}

type wpushSub struct {
	device, user string
	sub          proto.PushSubscription
	vapid        proto.VAPIDKeys
	events       []string
}

type wpushEvent struct {
	kind, who, title string
	users            []string
}

type wpushPut struct{ sub wpushSub }

func (w wpushPut) Put(tx *store.Tx) error {
	_, err := tx.Exec(
		"INSERT OR REPLACE INTO push_sub(device, user, sub, vapid, events) VALUES(?, ?, ?, ?, ?)",
		w.sub.device, w.sub.user, string(briefJSON(w.sub.sub)), string(briefJSON(w.sub.vapid)), string(briefJSON(w.sub.events)),
	)
	return err
}

type wpushDrop struct{ device string }

func (w wpushDrop) Put(tx *store.Tx) error {
	_, err := tx.Exec("DELETE FROM push_sub WHERE device = ?", w.device)
	return err
}

func wpushFor(home string) *wpushHome {
	h := wpushHomes[home]
	if h == nil {
		h = &wpushHome{subs: map[string]wpushSub{}}
		wpushHomes[home] = h
	}
	return h
}

func (l *Loop) wpushLoad() error {
	loaded := map[string]wpushSub{}
	read := func(tx *store.Tx) error { return wpushReadSubs(tx, loaded) }
	if err := l.cfg.Store.Write(read); err != nil {
		return err
	}
	wpushMu.Lock()
	h := wpushFor(l.cfg.Home)
	h.subs, h.loop = loaded, l
	wpushMu.Unlock()
	return nil
}

func (l *Loop) wpushClose() {
	wpushMu.Lock()
	if h := wpushFor(l.cfg.Home); h.loop == l {
		h.loop = nil
	}
	wpushMu.Unlock()
}

func wpushReadSubs(tx *store.Tx, dst map[string]wpushSub) error {
	rows, err := tx.Query("SELECT device, user, sub, vapid, events FROM push_sub")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var sub wpushSub
		var rawSub, rawVAPID, rawEvents string
		rows.Scan(&sub.device, &sub.user, &rawSub, &rawVAPID, &rawEvents)
		json.Unmarshal([]byte(rawSub), &sub.sub)
		json.Unmarshal([]byte(rawVAPID), &sub.vapid)
		json.Unmarshal([]byte(rawEvents), &sub.events)
		dst[sub.device] = sub
	}
	return rows.Err()
}

func wpushCmd(_ *State, c Command) (Result, *Refusal) {
	var off proto.PushOff
	if c.Decode(&off) == nil {
		return Result{Writes: []Write{wpushDrop{device: c.Who.Device}}}, nil
	}
	var on proto.PushOn
	if refusal := c.Decode(&on); refusal != nil {
		return Result{}, refusal
	}
	sub := wpushSub{device: c.Who.Device, user: c.Who.User, sub: on.Sub, vapid: on.VAPID, events: slices.Clone(on.On)}
	return Result{Writes: []Write{wpushPut{sub: sub}}}, nil
}

func wpushAfter(s *State, res Result) {
	for _, w := range res.Writes {
		wpushApplyWrite(s.Home, w)
	}
	for _, r := range res.Rows {
		if msg, ok := r.Value().(proto.MsgRow); ok {
			wpushNotify(s, msg)
		}
	}
}

func wpushApplyWrite(home string, w Write) {
	wpushMu.Lock()
	defer wpushMu.Unlock()
	h := wpushFor(home)
	switch w := w.(type) {
	case wpushPut:
		h.subs[w.sub.device] = w.sub
	case wpushDrop:
		delete(h.subs, w.device)
	case peopleForget:
		for device, sub := range h.subs {
			if sub.user == w.user {
				delete(h.subs, device)
			}
		}
	}
}

func wpushNotify(s *State, msg proto.MsgRow) {
	if msg.Pos != msg.Seq {
		return
	}
	ev := wpushExtract(s, msg)
	// slices.Clip sets cap == len so webpush-go allocates its own buffer
	// instead of writing the padding delimiter into shared spare capacity.
	payload := slices.Clip([]byte(briefJSON(proto.Reminder{
		Group: proto.ID(s.ID), Msg: proto.ID(msg.ID), GroupName: s.Group.Name,
		Kind: ev.kind, Who: ev.who, Title: ev.title,
	})))
	wpushSend(s, ev, payload)
}

func wpushExtract(s *State, msg proto.MsgRow) wpushEvent {
	if extract, ok := wpushExtractors[msg.Kind]; ok {
		return extract(s, msg)
	}
	return wpushMention(s, msg)
}

func wpushApproval(s *State, msg proto.MsgRow) wpushEvent {
	var b proto.ApprovalBody
	json.Unmarshal(msg.Body, &b)
	title := ""
	if b.Why != nil {
		title = wpushTitle(*b.Why)
	}
	return wpushEvent{kind: "approval", who: wpushWho(s, msg.By), title: title, users: b.Approvers}
}

func wpushDecision(s *State, msg proto.MsgRow) wpushEvent {
	var b proto.DecisionBody
	json.Unmarshal(msg.Body, &b)
	return wpushEvent{kind: "decision", who: wpushWho(s, msg.By), title: wpushTitle(b.Question), users: b.To}
}

func wpushBrief(s *State, msg proto.MsgRow) wpushEvent {
	var b proto.BriefBody
	json.Unmarshal(msg.Body, &b)
	var users []string
	if p, ok := s.People[b.Requester]; ok {
		users = []string{p.User}
	}
	return wpushEvent{kind: "brief", who: wpushWho(s, msg.By), title: wpushTitle(b.Title), users: users}
}

func wpushSummary(s *State, msg proto.MsgRow) wpushEvent {
	var b proto.SummaryBody
	json.Unmarshal(msg.Body, &b)
	users := make([]string, 0, len(s.People))
	for _, p := range s.People {
		users = append(users, p.User)
	}
	return wpushEvent{kind: "done", who: wpushWho(s, msg.By), title: wpushTitle(s.Tasks[b.Task].Title), users: users}
}

func wpushMention(s *State, msg proto.MsgRow) wpushEvent {
	var users []string
	for _, id := range msg.Mentions {
		if p, ok := s.People[id]; ok {
			users = append(users, p.User)
		}
	}
	return wpushEvent{kind: "mention", who: wpushWho(s, msg.By), users: users}
}

func wpushWho(s *State, member string) string {
	if ai, ok := s.AIs[member]; ok {
		return ai.Name
	}
	return s.People[member].Name
}

func wpushTitle(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	line, _, _ = strings.Cut(line, "\r")
	runes := []rune(line)
	return string(runes[:min(len(runes), wpushTitleLimit)])
}

func wpushTargets(s *State, subs map[string]wpushSub, ev wpushEvent) []wpushSub {
	var out []wpushSub
	for _, dev := range slices.Sorted(maps.Keys(subs)) {
		if sub := subs[dev]; wpushEligible(s, sub, ev) {
			out = append(out, sub)
		}
	}
	return out
}

func wpushEligible(s *State, sub wpushSub, ev wpushEvent) bool {
	return !s.Online[sub.device] &&
		slices.Contains(sub.events, ev.kind) &&
		slices.Contains(ev.users, sub.user) &&
		s.user(sub.device) == sub.user
}

func wpushSend(s *State, ev wpushEvent, payload []byte) {
	wpushMu.Lock()
	h := wpushFor(s.Home)
	loop, targets := h.loop, wpushTargets(s, h.subs, ev)
	loop.wg.Add(len(targets))
	wpushMu.Unlock()
	for _, sub := range targets {
		go wpushDeliver(h, loop, sub, payload)
	}
}

func wpushDeliver(h *wpushHome, loop *Loop, sub wpushSub, payload []byte) {
	defer loop.wg.Done()
	ctx, cancel := context.WithTimeout(context.Background(), wpushWindow)
	defer cancel()
	dst := &webpush.Subscription{Endpoint: sub.sub.Endpoint, Keys: webpush.Keys{P256dh: sub.sub.P256DH, Auth: sub.sub.Auth}}
	opts := &webpush.Options{Subscriber: wpushSubscriber, TTL: wpushTTL, VAPIDPublicKey: sub.vapid.Public, VAPIDPrivateKey: sub.vapid.Private}
	resp, err := webpush.SendNotificationWithContext(ctx, payload, dst, opts)
	if err != nil {
		return
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		wpushEvict(h, loop, sub)
	}
}

func wpushEvict(h *wpushHome, loop *Loop, stale wpushSub) {
	wpushMu.Lock()
	if h.subs[stale.device].sub == stale.sub {
		delete(h.subs, stale.device)
	}
	wpushMu.Unlock()
	drop := func(tx *store.Tx) error {
		_, err := tx.Exec("DELETE FROM push_sub WHERE device = ? AND sub = ?", stale.device, string(briefJSON(stale.sub)))
		return err
	}
	if err := loop.cfg.Store.Write(drop); err != nil {
		loop.cfg.Report(err)
	}
}
