package home_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

var (
	postA     = id(60)
	postB     = id(61)
	postGuest = id(62)
	postTask  = id(63)
	postPrev  = id(64)
)

const postTS = clock - 400

func postState(t *testing.T) *home.State {
	t.Helper()
	s := group(t)
	s.AIs[postA] = ai(postA, laptop)
	s.AIs[postB] = ai(postB, laptop)
	s.People[postGuest] = person(postGuest, guest, "member")
	s.Tasks[postTask] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: postTask, Seq: 8}, Group: groupID, Num: 1,
		Title: "Show path", Slug: proto.MakeSlug("Show path"), Origin: "brief", Brief: id(65),
		State: "active", Step: "local_check", Assignee: postA, Reviewer: postB, Pass: 90,
		Depends: []string{}, Thread: proto.ThreadSummary{Count: 3, Rounds: 2},
		Stats: proto.TaskStats{Started: clock - 5000}, Created: clock - 5000,
	}
	return s
}

func postMake[B any](n byte, as, kind string, thread *string, mentions []string, re *string, text string, body B) proto.Post[B] {
	ids := make([]proto.ID, len(mentions))
	for i, m := range mentions {
		ids[i] = proto.ID(m)
	}
	return proto.Post[B]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: proto.ID(id(n)), Timestamp: postTS, Group: proto.ID(groupID), As: proto.ID(as),
		},
		Kind: kind, Thread: (*proto.ID)(thread), Text: text, Mentions: ids, Re: (*proto.ID)(re), Body: body,
	}
}

func postSend(t *testing.T, s *home.State, d home.Disk, by identity.Key, payload any) (proto.Envelope, home.Result) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	env := by.Sign(string(data))
	res, err := home.Commands.Apply(s, d, receipts{}, env, now)
	if err != nil {
		t.Fatal(err)
	}
	return env, res
}

func postRawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

func postCheckNulls(t *testing.T, row any, allowed ...string) {
	t.Helper()
	equal(t, proto.UnexpectedNulls(postRawJSON(t, row), allowed...), []string{})
}

func postAssertRefused(t *testing.T, res home.Result, code string) {
	t.Helper()
	if res.Answer.Code != code || res.Msg == "" || len(res.Rows) != 0 || len(res.Items) != 0 {
		t.Fatalf("answered %+v (%q) with %d rows and %d items; want %s", res.Answer, res.Msg, len(res.Rows), len(res.Items), code)
	}
}

func postExpectedMsg(n byte, seq int64, by, kind string, thread *string, text string, mentions []string, re *string, body json.RawMessage, env proto.Envelope) proto.MsgRow {
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(n), Seq: seq}, Pos: seq, Group: groupID,
		Thread: thread, By: by, Kind: kind, Timestamp: postTS, Text: text, Mentions: mentions,
		Re: re, Body: body, Env: &env,
	}
}

func TestPostTextMainLineAndSecretaryRouting(t *testing.T) {
	t.Run("a person in the main line with no mentions wakes the secretary", func(t *testing.T) {
		s := postState(t)
		env, res := postSend(t, s, disk{}, browser, postMake(70, "", "text", nil, nil, nil, "Check <paths> & relays.", proto.EmptyBody{}))
		want := postExpectedMsg(70, 11, ownerID, "text", nil, "Check <paths> & relays.", []string{}, nil, json.RawMessage(`{}`), env)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, []any{res.Rows[0].Value()}, []any{want})
		equal(t, res.Items, []home.Item{{AI: aiID, Why: "mention", Msg: id(70)}})
		postCheckNulls(t, res.Rows[0].Value(), "/thread", "/re", "/state")
	})
	t.Run("a person mentioning only a person in the main line still wakes the secretary", func(t *testing.T) {
		s := postState(t)
		re := postPrev
		env, res := postSend(t, s, disk{}, browser, postMake(71, "", "text", nil, []string{postGuest}, &re, "See @guest", proto.EmptyBody{}))
		want := postExpectedMsg(71, 11, ownerID, "text", nil, "See @guest", []string{postGuest}, &re, json.RawMessage(`{}`), env)
		equal(t, []any{res.Rows[0].Value()}, []any{want})
		equal(t, res.Items, []home.Item{{AI: aiID, Why: "mention", Msg: id(71)}})
		postCheckNulls(t, res.Rows[0].Value(), "/thread", "/state")
	})
	t.Run("a person mentioning an AI wakes only that AI", func(t *testing.T) {
		s := postState(t)
		_, res := postSend(t, s, disk{}, browser, postMake(72, "", "text", nil, []string{postA}, nil, "Ask @a", proto.EmptyBody{}))
		equal(t, res.Items, []home.Item{{AI: postA, Why: "mention", Msg: id(72)}})
	})
	t.Run("without a secretary a main-line text with no AI mention queues nothing", func(t *testing.T) {
		s := postState(t)
		s.Group.Secretary = ""
		_, res := postSend(t, s, disk{}, browser, postMake(73, "", "text", nil, nil, nil, "Hello", proto.EmptyBody{}))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Items, ([]home.Item)(nil))
	})
	t.Run("an AI in the main line with no AI mention wakes nobody", func(t *testing.T) {
		s := postState(t)
		_, res := postSend(t, s, disk{}, laptop, postMake(74, postA, "text", nil, []string{ownerID}, nil, "Done.", proto.EmptyBody{}))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Items, ([]home.Item)(nil))
	})
	t.Run("an AI mentioning two AIs and a person wakes both AIs in order", func(t *testing.T) {
		s := postState(t)
		_, res := postSend(t, s, disk{}, laptop, postMake(75, postA, "text", nil, []string{postGuest, aiID, postB}, nil, "Heads up", proto.EmptyBody{}))
		equal(t, res.Items, []home.Item{
			{AI: aiID, Why: "mention", Msg: id(75)},
			{AI: postB, Why: "mention", Msg: id(75)},
		})
	})
	t.Run("an AI mentioning only itself queues no item", func(t *testing.T) {
		s := postState(t)
		_, res := postSend(t, s, disk{}, laptop, postMake(76, aiID, "text", nil, []string{aiID}, nil, "Note to self", proto.EmptyBody{}))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Rows[0].Value().(proto.MsgRow).Mentions, []string{aiID})
		equal(t, res.Items, ([]home.Item)(nil))
	})
}

func TestPostThreadSummaryAndPreviewBoundary(t *testing.T) {
	s := postState(t)
	thread := postTask
	exact200 := strings.Repeat("世", 200)
	env, res := postSend(t, s, disk{}, browser, postMake(80, "", "text", &thread, nil, nil, exact200, proto.EmptyBody{}))
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, res.Items, ([]home.Item)(nil))
	wantMsg := postExpectedMsg(80, 11, ownerID, "text", &thread, exact200, []string{}, nil, json.RawMessage(`{}`), env)
	wantTask := s.Tasks[postTask]
	wantTask.Seq = 12
	wantTask.Thread = proto.ThreadSummary{
		Count: 4, Rounds: 2,
		Last: &proto.ThreadMessage{By: ownerID, Kind: "text", Text: exact200, Timestamp: postTS},
	}
	equal(t, []any{res.Rows[0].Value(), res.Rows[1].Value()}, []any{wantMsg, wantTask})
	postCheckNulls(t, res.Rows[0].Value(), "/re", "/state")
	postCheckNulls(t, res.Rows[1].Value(), "/wait", "/stats/ended")
	s.Commit(res)

	over200 := exact200 + "界"
	_, res = postSend(t, s, disk{}, browser, postMake(81, "", "text", &thread, []string{postB}, nil, over200, proto.EmptyBody{}))
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, res.Rows[0].Value().(proto.MsgRow).Text, over200)
	updated := res.Rows[1].Value().(proto.TaskRow)
	equal(t, updated.Thread, proto.ThreadSummary{
		Count: 5, Rounds: 2,
		Last: &proto.ThreadMessage{By: ownerID, Kind: "text", Text: exact200, Timestamp: postTS},
	})
	equal(t, res.Items, []home.Item{{AI: postB, Why: "mention", Task: postTask, Msg: id(81)}})
	s.Commit(res)

	_, res = postSend(t, s, disk{}, browser, postMake(82, "", "text", &thread, nil, nil, "", proto.EmptyBody{}))
	equal(t, res.Rows[1].Value().(proto.TaskRow).Thread.Last.Text, "")
}

func TestPostProgressAndQuestion(t *testing.T) {
	s := postState(t)
	thread := postTask
	env, res := postSend(t, s, disk{}, laptop, postMake(85, postB, "progress", &thread, []string{postGuest, postB, postA}, nil, "Checked diff.", proto.EmptyBody{}))
	equal(t, res.Answer, store.Outcome{OK: true})
	wantMsg := postExpectedMsg(85, 11, postB, "progress", &thread, "Checked diff.", []string{postGuest, postB, postA}, nil, json.RawMessage(`{}`), env)
	equal(t, res.Rows[0].Value(), any(wantMsg))
	equal(t, res.Rows[1].Value().(proto.TaskRow).Thread.Last, &proto.ThreadMessage{By: postB, Kind: "progress", Text: "Checked diff.", Timestamp: postTS})
	equal(t, res.Items, []home.Item{{AI: postA, Why: "mention", Task: postTask, Msg: id(85)}})
	postCheckNulls(t, res.Rows[0].Value(), "/re", "/state")
	s.Commit(res)

	env, res = postSend(t, s, disk{}, laptop, postMake(86, aiID, "question", nil, []string{postA}, nil, "Include UDP?", proto.EmptyBody{}))
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, len(res.Rows), 1)
	equal(t, res.Rows[0].Value(), any(postExpectedMsg(86, 13, aiID, "question", nil, "Include UDP?", []string{postA}, nil, json.RawMessage(`{}`), env)))
	equal(t, res.Items, []home.Item{{AI: postA, Why: "mention", Msg: id(86)}})
	postCheckNulls(t, res.Rows[0].Value(), "/thread", "/re", "/state")
	s.Commit(res)

	q := postMake(87, postB, "question", &thread, nil, nil, "Ready?", proto.EmptyBody{})
	q.Main = true
	_, res = postSend(t, s, disk{}, laptop, q)
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, len(res.Rows), 2)
	equal(t, res.Rows[0].Value().(proto.MsgRow).Main, false)
	equal(t, res.Items, ([]home.Item)(nil))
}

func TestPostReport(t *testing.T) {
	s := postState(t)
	thread := postTask
	bare := proto.ReportBody{Commit: "9f2c1ab", Files: 4, Added: 212, Removed: 18}
	env, res := postSend(t, s, disk{}, laptop, postMake(90, postA, "report", &thread, []string{postB}, nil, "All green.", bare))
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, res.Items, ([]home.Item)(nil))
	wantMsg := postExpectedMsg(90, 11, postA, "report", &thread, "All green.", []string{postB}, nil, postRawJSON(t, bare), env)
	equal(t, res.Rows[0].Value(), any(wantMsg))
	equal(t, res.Rows[1].Value().(proto.TaskRow).Thread.Last, &proto.ThreadMessage{By: postA, Kind: "report", Text: "All green.", Timestamp: postTS})
	postCheckNulls(t, res.Rows[0].Value(), "/re", "/state")
	s.Commit(res)

	pr := 31
	withPR := proto.ReportBody{Commit: "9f2c1ab", Files: 4, Added: 212, Removed: 18, PR: &pr}
	_, res = postSend(t, s, disk{}, laptop, postMake(91, postA, "report", &thread, nil, nil, "", withPR))
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, res.Rows[0].Value().(proto.MsgRow).Body, postRawJSON(t, withPR))
}

type postRefusalEnv struct {
	s       *home.State
	by      identity.Key
	payload string
}

func postJSONText(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestPostRefusals(t *testing.T) {
	thread, missing := postTask, id(99)
	report := proto.ReportBody{Commit: "9f2c1ab", Files: 1, Added: 2, Removed: 0}

	cases := map[string]struct {
		code  string
		setup func(*postRefusalEnv)
	}{
		"progress by a person": {code: "forbidden", setup: func(e *postRefusalEnv) {
			e.by, e.payload = browser, postJSONText(postMake(95, "", "progress", &thread, nil, nil, "x", proto.EmptyBody{}))
		}},
		"question by a person": {code: "forbidden", setup: func(e *postRefusalEnv) {
			e.by, e.payload = browser, postJSONText(postMake(95, "", "question", nil, nil, nil, "x", proto.EmptyBody{}))
		}},
		"report by a person": {code: "forbidden", setup: func(e *postRefusalEnv) {
			e.by, e.payload = browser, postJSONText(postMake(95, "", "report", &thread, nil, nil, "", report))
		}},
		"progress in the main line": {code: "bad_request", setup: func(e *postRefusalEnv) {
			e.payload = postJSONText(postMake(95, postA, "progress", nil, nil, nil, "x", proto.EmptyBody{}))
		}},
		"report in the main line": {code: "bad_request", setup: func(e *postRefusalEnv) {
			e.payload = postJSONText(postMake(95, postA, "report", nil, nil, nil, "", report))
		}},
		"text with bad re id": {code: "bad_request", setup: func(e *postRefusalEnv) {
			bad := "not-an-id"
			e.by, e.payload = browser, postJSONText(postMake(95, "", "text", nil, nil, &bad, "x", proto.EmptyBody{}))
		}},
		"report missing commit": {code: "bad_request", setup: func(e *postRefusalEnv) {
			raw := postJSONText(postMake(95, postA, "report", &thread, nil, nil, "", report))
			e.payload = strings.Replace(raw, `"commit"`, `"sha"`, 1)
		}},
		"mentioning a member twice": {code: "bad_request", setup: func(e *postRefusalEnv) {
			e.by, e.payload = browser, postJSONText(postMake(95, "", "text", nil, []string{postA, postA}, nil, "x", proto.EmptyBody{}))
		}},
		"mentioning two, then one again": {code: "bad_request", setup: func(e *postRefusalEnv) {
			e.payload = postJSONText(postMake(95, postA, "question", nil, []string{postA, postB, postA}, nil, "x", proto.EmptyBody{}))
		}},
		"text in an unknown thread": {code: "not_found", setup: func(e *postRefusalEnv) {
			e.by, e.payload = browser, postJSONText(postMake(95, "", "text", &missing, nil, nil, "x", proto.EmptyBody{}))
		}},
		"progress in an unknown thread": {code: "not_found", setup: func(e *postRefusalEnv) {
			e.payload = postJSONText(postMake(95, postA, "progress", &missing, nil, nil, "x", proto.EmptyBody{}))
		}},
		"report in an unknown thread": {code: "not_found", setup: func(e *postRefusalEnv) {
			e.payload = postJSONText(postMake(95, postA, "report", &missing, nil, nil, "", report))
		}},
		"report by the reviewer": {code: "forbidden", setup: func(e *postRefusalEnv) {
			e.payload = postJSONText(postMake(95, postB, "report", &thread, nil, nil, "", report))
		}},
		"report by the secretary": {code: "forbidden", setup: func(e *postRefusalEnv) {
			e.payload = postJSONText(postMake(95, aiID, "report", &thread, nil, nil, "", report))
		}},
		"mentioning no member of the group": {code: "conflict", setup: func(e *postRefusalEnv) {
			e.by, e.payload = browser, postJSONText(postMake(95, "", "text", nil, []string{missing}, nil, "x", proto.EmptyBody{}))
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := postRefusalEnv{s: postState(t), by: laptop}
			tc.setup(&env)
			res, err := home.Commands.Apply(env.s, disk{}, receipts{}, env.by.Sign(env.payload), now)
			if err != nil {
				t.Fatal(err)
			}
			postAssertRefused(t, res, tc.code)
		})
	}
}

func TestPostHandback(t *testing.T) {
	thread := postTask
	facts := proto.HandbackBody{Files: []string{"a.go"}, Commits: []proto.CommitSubject{{SHA: "9f2c1ab", Subject: "Fix"}}, Added: 40, Removed: 3}
	t.Run("an AI in a thread notifies the secretary", func(t *testing.T) {
		s := postState(t)
		env, res := postSend(t, s, disk{}, laptop, postMake(100, postA, "handback", &thread, nil, nil, "Owner asked for X.", facts))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Rows[0].Value(), any(postExpectedMsg(100, 11, postA, "handback", &thread, "Owner asked for X.", []string{}, nil, postRawJSON(t, facts), env)))
		equal(t, res.Rows[1].Value().(proto.TaskRow).Thread.Last.Kind, "handback")
		equal(t, res.Items, []home.Item{{AI: aiID, Why: "notice", Task: postTask, Msg: id(100), Code: "handback"}})
	})
	t.Run("the secretary posts in the main line and notifies nobody", func(t *testing.T) {
		s := postState(t)
		_, res := postSend(t, s, disk{}, laptop, postMake(101, aiID, "handback", nil, nil, nil, "", proto.HandbackBody{Files: []string{}, Commits: []proto.CommitSubject{}}))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, len(res.Rows), 1)
		equal(t, res.Items, ([]home.Item)(nil))
	})
	t.Run("without a secretary a thread hand-back notifies nobody", func(t *testing.T) {
		s := postState(t)
		s.Group.Secretary = ""
		_, res := postSend(t, s, disk{}, laptop, postMake(102, postA, "handback", &thread, nil, nil, "", facts))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Items, ([]home.Item)(nil))
	})
	t.Run("another AI in the main line is refused", func(t *testing.T) {
		s := postState(t)
		_, res := postSend(t, s, disk{}, laptop, postMake(103, postA, "handback", nil, nil, nil, "", facts))
		postAssertRefused(t, res, "bad_request")
	})
	t.Run("a person is refused", func(t *testing.T) {
		s := postState(t)
		_, res := postSend(t, s, disk{}, browser, postMake(104, "", "handback", &thread, nil, nil, "", facts))
		postAssertRefused(t, res, "forbidden")
	})
}
