package home

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Web Push subscriptions and encrypted short reminders for browsers that are
// offline when an event concerns their person (planning/contracts/commands.md
// section 1.7; planning/contracts/storage.md section 1, push_sub).
//
// Concurrency and lock order: wpushMu is a leaf lock protecting wpushSubs,
// wpushQueue and wpushRunning. Nothing held under wpushMu calls Loop, Store,
// Disk or HTTP, so it never closes a cycle with Loop.mu or the store writer.

func init() {
	Commands.Add("push", wpushCmd)
	After.Add("push", wpushAfter)
}

const (
	wpushAuthLen    = 16
	wpushTitleLimit = 80
	wpushWorkers    = 4
	wpushQueueCap   = 64
	wpushTTL        = 86400
	wpushSubscriber = "https://solodawn.example"
)

var (
	wpushKinds = map[string]bool{
		"mention":  true,
		"approval": true,
		"decision": true,
		"brief":    true,
		"done":     true,
	}

	wpushExtractors = map[string]func(*State, proto.MsgRow) (wpushEvent, bool){
		"approval": wpushApproval,
		"decision": wpushDecision,
		"brief":    wpushBrief,
		"summary":  wpushSummary,
	}

	wpushMu      sync.Mutex
	wpushSubs    = map[string]map[string]wpushSub{} // home device id -> browser device id -> subscription
	wpushQueue   []wpushJob
	wpushRunning int
	wpushBusy    sync.WaitGroup

	wpushTimeout                    = 5 * time.Second
	wpushClient  webpush.HTTPClient = &http.Client{Timeout: wpushTimeout}
)

type wpushSub struct {
	device string
	user   string
	sub    proto.PushSubscription
	vapid  proto.VAPIDKeys
	events []string
}

type wpushJob struct {
	home    string
	sub     wpushSub
	payload []byte
}

type wpushEvent struct {
	kind  string
	who   string
	title string
	users []string
}

type wpushPut struct {
	sub wpushSub
}

func (w wpushPut) Put(tx *store.Tx) error {
	_, err := tx.Exec(
		"INSERT OR REPLACE INTO push_sub(device, user, sub, vapid, events) VALUES(?, ?, ?, ?, ?)",
		w.sub.device,
		w.sub.user,
		string(wpushMarshal(w.sub.sub)),
		string(wpushMarshal(w.sub.vapid)),
		string(wpushMarshal(w.sub.events)),
	)
	return err
}

type wpushDrop struct {
	device string
}

func (w wpushDrop) Put(tx *store.Tx) error {
	_, err := tx.Exec("DELETE FROM push_sub WHERE device = ?", w.device)
	return err
}

// LoadPushSubscriptions reads the persisted browser push subscriptions from
// the loop's store into memory for its home, replacing any subscriptions held
// for that home before.
func (l *Loop) LoadPushSubscriptions() error {
	loaded := map[string]wpushSub{}
	read := func(tx *store.Tx) error { return wpushReadSubs(tx, loaded) }
	if err := l.cfg.Store.Write(read); err != nil {
		return err
	}
	wpushMu.Lock()
	wpushSubs[l.cfg.Home] = loaded
	wpushMu.Unlock()
	return nil
}

func wpushReadSubs(tx *store.Tx, dst map[string]wpushSub) error {
	rows, err := tx.Query("SELECT device, user, sub, vapid, events FROM push_sub")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		sub, err := wpushScanSub(rows)
		if err != nil {
			return err
		}
		dst[sub.device] = sub
	}
	return rows.Err()
}

func wpushScanSub(rows *sql.Rows) (sub wpushSub, err error) {
	var rawSub, rawVAPID, rawEvents string
	if err := rows.Scan(&sub.device, &sub.user, &rawSub, &rawVAPID, &rawEvents); err != nil {
		return sub, err
	}
	err = errors.Join(
		proto.Decode([]byte(rawSub), &sub.sub),
		proto.Decode([]byte(rawVAPID), &sub.vapid),
		proto.Decode([]byte(rawEvents), &sub.events),
	)
	return sub, err
}

func wpushCmd(s *State, c Command) (Result, *Refusal) {
	if !wpushSignerOK(s, c.Who) {
		return Result{}, &Refusal{"forbidden", "only a browser of a member may set its push subscription"}
	}
	var off proto.PushOff
	if c.Decode(&off) == nil {
		return Result{Writes: []Write{wpushDrop{device: c.Who.Device}}}, nil
	}
	var on proto.PushOn
	if refusal := c.Decode(&on); refusal != nil {
		return Result{}, refusal
	}
	if fault := wpushValid(on); fault != "" {
		return Result{}, &Refusal{"bad_request", fault}
	}
	sub := wpushSub{
		device: c.Who.Device,
		user:   c.Who.User,
		sub:    on.Sub,
		vapid:  on.VAPID,
		events: append([]string{}, on.On...),
	}
	return Result{Writes: []Write{wpushPut{sub: sub}}}, nil
}

func wpushSignerOK(s *State, w Who) bool {
	return w.AI == "" && wpushBrowser(s.Logs[w.User], w.Device)
}

func wpushBrowser(log *identity.Log, device string) bool {
	if log == nil || !log.Valid(device) {
		return false
	}
	kind := ""
	for _, env := range log.Entries {
		var e struct {
			Device string `json:"dev"`
			Kind   string `json:"kind,omitempty"`
		}
		proto.Decode([]byte(env.Payload), &e)
		if e.Device == device {
			kind = e.Kind
		}
	}
	return kind == "browser"
}

func wpushValid(on proto.PushOn) string {
	if !wpushEndpointOK(on.Sub.Endpoint) || !wpushSubKeysOK(on.Sub) {
		return "push subscription endpoint or keys are invalid"
	}
	if !wpushVAPIDOK(on.VAPID) {
		return "push VAPID key pair is invalid"
	}
	if !wpushEventsOK(on.On) {
		return "push events must be distinct known reminder kinds"
	}
	return ""
}

func wpushEndpointOK(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

func wpushBytes(s string) ([]byte, bool) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	return b, err == nil && base64.RawURLEncoding.EncodeToString(b) == s
}

func wpushSubKeysOK(sub proto.PushSubscription) bool {
	dh, okDH := wpushBytes(sub.P256DH)
	auth, okAuth := wpushBytes(sub.Auth)
	_, err := ecdh.P256().NewPublicKey(dh)
	return okDH && err == nil && okAuth && len(auth) == wpushAuthLen
}

func wpushVAPIDOK(v proto.VAPIDKeys) bool {
	pub, okPub := wpushBytes(v.Public)
	privBytes, okPriv := wpushBytes(v.Private)
	priv, err := ecdh.P256().NewPrivateKey(privBytes)
	return okPub && okPriv && err == nil && bytes.Equal(priv.PublicKey().Bytes(), pub)
}

func wpushEventsOK(events []string) bool {
	for i, kind := range events {
		if !wpushKinds[kind] || slices.Contains(events[:i], kind) {
			return false
		}
	}
	return true
}

func wpushAfter(s *State, res Result) {
	for _, w := range res.Writes {
		wpushApplyWrite(s.Home, w)
	}
	for _, r := range res.Rows {
		if msg, ok := r.Value().(proto.MsgRow); ok && msg.Pos == msg.Seq {
			wpushNotify(s, msg)
		}
	}
}

func wpushApplyWrite(home string, w Write) {
	wpushMu.Lock()
	defer wpushMu.Unlock()
	switch w := w.(type) {
	case wpushPut:
		if wpushSubs[home] == nil {
			wpushSubs[home] = map[string]wpushSub{}
		}
		wpushSubs[home][w.sub.device] = w.sub
	case wpushDrop:
		delete(wpushSubs[home], w.device)
	}
}

func wpushNotify(s *State, msg proto.MsgRow) {
	ev, ok := wpushExtract(s, msg)
	if !ok {
		return
	}
	payload := wpushMarshal(proto.Reminder{
		Group:     proto.ID(s.ID),
		Msg:       proto.ID(msg.ID),
		GroupName: s.Group.Name,
		Kind:      ev.kind,
		Who:       ev.who,
		Title:     ev.title,
	})
	for _, sub := range wpushTargets(s, ev) {
		wpushEnqueue(wpushJob{home: s.Home, sub: sub, payload: payload})
	}
}

func wpushExtract(s *State, msg proto.MsgRow) (wpushEvent, bool) {
	if extract, ok := wpushExtractors[msg.Kind]; ok {
		return extract(s, msg)
	}
	return wpushMention(s, msg)
}

func wpushPending(msg proto.MsgRow) bool {
	return msg.State != nil && msg.State.Status == "pending"
}

func wpushApproval(s *State, msg proto.MsgRow) (wpushEvent, bool) {
	if !wpushPending(msg) {
		return wpushEvent{}, false
	}
	var b proto.ApprovalBody
	json.Unmarshal(msg.Body, &b)
	title := ""
	if b.Why != nil {
		title = wpushTitle(*b.Why)
	}
	return wpushEvent{kind: "approval", who: wpushWho(s, msg.By), title: title, users: b.Approvers}, true
}

func wpushDecision(s *State, msg proto.MsgRow) (wpushEvent, bool) {
	if !wpushPending(msg) {
		return wpushEvent{}, false
	}
	var b proto.DecisionBody
	json.Unmarshal(msg.Body, &b)
	return wpushEvent{kind: "decision", who: wpushWho(s, msg.By), title: wpushTitle(b.Question), users: b.To}, true
}

func wpushBrief(s *State, msg proto.MsgRow) (wpushEvent, bool) {
	if !wpushPending(msg) {
		return wpushEvent{}, false
	}
	var b proto.BriefBody
	json.Unmarshal(msg.Body, &b)
	var users []string
	if p, ok := s.People[b.Requester]; ok {
		users = []string{p.User}
	}
	return wpushEvent{kind: "brief", who: wpushWho(s, msg.By), title: wpushTitle(b.Title), users: users}, true
}

func wpushSummary(s *State, msg proto.MsgRow) (wpushEvent, bool) {
	var b proto.SummaryBody
	json.Unmarshal(msg.Body, &b)
	users := make([]string, 0, len(s.People))
	for _, p := range s.People {
		users = append(users, p.User)
	}
	return wpushEvent{kind: "done", who: wpushWho(s, msg.By), title: wpushTitle(s.Tasks[b.Task].Title), users: users}, true
}

func wpushMention(s *State, msg proto.MsgRow) (wpushEvent, bool) {
	var users []string
	for _, id := range msg.Mentions {
		if p, ok := s.People[id]; ok {
			users = append(users, p.User)
		}
	}
	return wpushEvent{kind: "mention", who: wpushWho(s, msg.By), users: users}, true
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

func wpushTargets(s *State, ev wpushEvent) []wpushSub {
	wpushMu.Lock()
	defer wpushMu.Unlock()
	var out []wpushSub
	for _, dev := range slices.Sorted(maps.Keys(wpushSubs[s.Home])) {
		sub := wpushSubs[s.Home][dev]
		if wpushEligible(s, sub, ev) {
			out = append(out, sub)
		}
	}
	return out
}

func wpushEligible(s *State, sub wpushSub, ev wpushEvent) bool {
	return !s.Online[sub.device] &&
		slices.Contains(sub.events, ev.kind) &&
		slices.Contains(ev.users, sub.user) &&
		wpushMember(s, sub.user) &&
		wpushBrowser(s.Logs[sub.user], sub.device)
}

func wpushMember(s *State, user string) bool {
	for _, p := range s.People {
		if p.User == user {
			return true
		}
	}
	return false
}

func wpushEnqueue(job wpushJob) {
	wpushMu.Lock()
	if len(wpushQueue) >= wpushQueueCap {
		wpushMu.Unlock()
		return
	}
	wpushQueue = append(wpushQueue, job)
	start := wpushRunning < wpushWorkers
	if start {
		wpushRunning++
		wpushBusy.Add(1)
	}
	wpushMu.Unlock()
	if start {
		go wpushWork()
	}
}

func wpushWork() {
	defer wpushBusy.Done()
	for {
		job, ok := wpushNext()
		if !ok {
			return
		}
		wpushDeliver(job)
	}
}

func wpushNext() (wpushJob, bool) {
	wpushMu.Lock()
	defer wpushMu.Unlock()
	if len(wpushQueue) == 0 {
		wpushRunning--
		return wpushJob{}, false
	}
	job := wpushQueue[0]
	wpushQueue = wpushQueue[1:]
	return job, true
}

func wpushDeliver(job wpushJob) {
	ctx, cancel := context.WithTimeout(context.Background(), wpushTimeout)
	defer cancel()
	resp, err := webpush.SendNotificationWithContext(ctx, job.payload, &webpush.Subscription{
		Endpoint: job.sub.sub.Endpoint,
		Keys:     webpush.Keys{P256dh: job.sub.sub.P256DH, Auth: job.sub.sub.Auth},
	}, &webpush.Options{
		HTTPClient:      wpushClient,
		Subscriber:      wpushSubscriber,
		TTL:             wpushTTL,
		VAPIDPublicKey:  job.sub.vapid.Public,
		VAPIDPrivateKey: job.sub.vapid.Private,
	})
	if err != nil {
		return
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		wpushEvict(job.home, job.sub)
	}
}

func wpushEvict(home string, stale wpushSub) {
	wpushMu.Lock()
	defer wpushMu.Unlock()
	if wpushSubs[home][stale.device].sub == stale.sub {
		delete(wpushSubs[home], stale.device)
	}
}

// wpushMarshal encodes val as JSON without HTML escaping. slices.Clip sets
// cap == len so webpush-go's bytes.NewBuffer(message) allocates its own buffer
// instead of writing the padding delimiter into shared spare capacity.
func wpushMarshal(val any) []byte {
	var raw bytes.Buffer
	enc := json.NewEncoder(&raw)
	enc.SetEscapeHTML(false)
	enc.Encode(val)
	return slices.Clip(bytes.TrimSuffix(raw.Bytes(), []byte("\n")))
}
