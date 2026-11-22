package host

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestKBToolsSchemasVisibilityAndInstructions(t *testing.T) {
	lists := loadGoldenToolLists(t)
	allLists := []proto.ToolList{lists.Secretary, lists.Group, lists.Review, lists.TaskLevel1, lists.TaskLevel23}
	for _, name := range []string{"kb_put", "kb_search"} {
		checkRegisteredTalkTool(t, lists.Secretary, name)
		for _, list := range allLists {
			goldenRaw, _ := json.Marshal(findGoldenTool(t, list, name).InputSchema)
			assertEqual(t, decodeJSONValue(t, Tools[name].Schema), decodeJSONValue(t, goldenRaw))
		}
	}

	callers := []Caller{
		talkCaller(GroupSession, talkSecID, "", true, Turn{}),
		talkCaller(GroupSession, talkAI1ID, "", false, Turn{}),
		talkCaller(ReviewSession, talkAI2ID, talkTask2ID, false, Turn{}),
		{Session: Session{SessionKey: SessionKey{Task: talkTask2ID}, Kind: TaskSession}, Level: 1},
		{Session: Session{SessionKey: SessionKey{Task: talkTask2ID}, Kind: TaskSession}, Level: 3},
	}
	for _, c := range callers {
		listed := ToolsFor(c)
		for _, name := range []string{"kb_put", "kb_search"} {
			if _, ok := listed[name]; !ok {
				t.Fatalf("tool %q missing for caller %+v", name, c)
			}
		}
	}

	sessions := []Session{
		{Kind: GroupSession},
		{Kind: TaskSession},
		{Kind: ReviewSession},
		{Kind: GroupSession, Secretary: true},
	}
	for _, s := range sessions {
		inst := Instructions(s)
		if lines := strings.Count(strings.TrimSuffix(inst, "\n"), "\n") + 1; lines >= 60 {
			t.Fatalf("%+v instructions = %d lines, want < 60", s, lines)
		}
		if !strings.Contains(inst, "kb_search") || !strings.Contains(inst, "kb_put") {
			t.Fatalf("%+v instructions missing kb guidance:\n%s", s, inst)
		}
	}
}

func TestKBPutTool(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)
	home := newTalkHomeStub(
		`{"re":50,"ok":true,"id":"docCreated00000000000A","replaced":false}`,
		`{"re":51,"ok":true,"id":"docCreated00000000000A","replaced":true}`,
	)
	h.Attach("home-device", home)

	caller := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{})

	// Secret check covers both name and text before signing or sending.
	for _, raw := range []string{
		`{"name":"sk-proj-EXAMPLEexampleEXAMPLEexample_EXAMPLE-00","text":"clean"}`,
		`{"name":"Decision","text":"ghp_0123456789abcdefghijklmnopqrstuvwxyz"}`,
	} {
		_, err := Tools["kb_put"].Call(t.Context(), h, caller, json.RawMessage(raw))
		assertErrorCode(t, err, "secret")
	}
	assertEqual(t, len(home.asked), 0)

	// Create returns id and replaced: false.
	outCreate := mustCallTalk[kbPutResult](t, h, caller, "kb_put", `{"name":"Decision: <writer>","text":"Home writes <all> & rows."}`)
	assertEqual(t, outCreate, kbPutResult{ID: "docCreated00000000000A", Replaced: false})
	rawCreate, _ := json.Marshal(outCreate)
	assertNoNulls(t, rawCreate)
	sentCreate := mustDecodeCmd[proto.Note](t, home.asked[0], "/task")
	assertEqual(t, sentCreate.Task, (*proto.ID)(nil))
	assertEqual(t, sentCreate.NoteType, "kb")
	assertEqual(t, sentCreate.Name, "Decision: <writer>")
	assertEqual(t, sentCreate.Text, "Home writes <all> & rows.")
	assertEqual(t, string(sentCreate.As), talkAI1ID)
	assertEqual(t, sentCreate.ID != sentCreate.Nonce, true)

	// Replace returns existing id and replaced: true.
	outReplace := mustCallTalk[kbPutResult](t, h, caller, "kb_put", `{"name":"Decision: <writer>","text":"Updated."}`)
	assertEqual(t, outReplace, kbPutResult{ID: "docCreated00000000000A", Replaced: true})

	for _, bad := range []string{`{}`, `{"name":"x"}`, `{"text":"x"}`, `{"name":null,"text":"x"}`, `{"name":"x","text":null}`, `[]`} {
		_, err := Tools["kb_put"].Call(t.Context(), h, caller, json.RawMessage(bad))
		assertErrorCode(t, err, "bad_request")
	}

	home.replies = append(home.replies, json.RawMessage(`{"re":52,"err":{"code":"bad_request","msg":"name too long"}}`))
	_, err := Tools["kb_put"].Call(t.Context(), h, caller, json.RawMessage(`{"name":"x","text":"y"}`))
	assertErrorCode(t, err, "bad_request")

	close(home.done)
	_, err = Tools["kb_put"].Call(t.Context(), h, caller, json.RawMessage(`{"name":"x","text":"y"}`))
	assertErrorCode(t, err, "home_unreachable")
}

func TestKBSearchTool(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)

	r1, _ := json.Marshal(proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: "k1____________________A", Seq: 40},
		Pos:       40, Group: talkGroupID, NoteType: "kb", Name: "Single writer", Text: "Home owns writes.", By: talkAI2ID, Timestamp: seamStart,
	})
	r2, _ := json.Marshal(proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: "k2____________________A", Seq: 35},
		Pos:       35, Group: talkGroupID, NoteType: "kb", Name: "路径", Text: "连接路径事件", By: "goneMember000000000000A", Timestamp: seamStart - 500,
	})
	hitPage, _ := json.Marshal(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 50, OK: true}, Rows: []json.RawMessage{r1, r2}, More: true})
	emptyPage, _ := json.Marshal(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 51, OK: true}, Rows: []json.RawMessage{}, More: false})

	home := newTalkHomeStub(string(hitPage), string(emptyPage), string(emptyPage))
	h.Attach("home-device", home)

	caller := talkCaller(GroupSession, talkSecID, "", true, Turn{})
	got := mustCallTalk[kbSearchResult](t, h, caller, "kb_search", `{"query":"writer 路径"}`)
	assertEqual(t, got, kbSearchResult{
		Documents: []kbDocumentItem{
			{ID: "k1____________________A", Name: "Single writer", Text: "Home owns writes.", By: "Codex", At: seamStart},
			{ID: "k2____________________A", Name: "路径", Text: "连接路径事件", By: "goneMember000000000000A", At: seamStart - 500},
		},
		More: true,
	})
	assertEqual(t, home.asked[0].(proto.FetchKind), proto.FetchKind{
		FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: 50}, Group: talkGroupID, Before: 0, Limit: 3},
		Kind:      "kb",
		Query:     "writer 路径",
	})

	emptyOut := mustCallTalk[kbSearchResult](t, h, caller, "kb_search", `{"query":"none","limit":1}`)
	assertEqual(t, emptyOut, kbSearchResult{Documents: []kbDocumentItem{}, More: false})
	encodedEmpty, _ := json.Marshal(emptyOut)
	assertEqual(t, string(encodedEmpty), `{"documents":[],"more":false}`)
	assertNoNulls(t, encodedEmpty)
	assertEqual(t, home.asked[1].(proto.FetchKind).Limit, 1)

	mustCallTalk[kbSearchResult](t, h, caller, "kb_search", `{"query":"max","limit":20}`)
	assertEqual(t, home.asked[2].(proto.FetchKind).Limit, 20)

	for _, bad := range []string{
		`{}`,
		`[]`,
		`{"query":null}`,
		`{"query":""}`,
		`{"query":"   \t\n "}`,
		`{"query":"ok","limit":0}`,
		`{"query":"ok","limit":-1}`,
		`{"query":"ok","limit":21}`,
		`{"query":"ok","limit":1.5}`,
	} {
		_, err := Tools["kb_search"].Call(t.Context(), h, caller, json.RawMessage(bad))
		assertErrorCode(t, err, "bad_request")
	}
	assertEqual(t, len(home.asked), 3)

	home.replies = append(home.replies, json.RawMessage(`{"re":53,"err":{"code":"forbidden","msg":"denied"}}`))
	_, err := Tools["kb_search"].Call(t.Context(), h, caller, json.RawMessage(`{"query":"ok"}`))
	assertErrorCode(t, err, "forbidden")

	home.err = io.EOF
	_, err = Tools["kb_search"].Call(t.Context(), h, caller, json.RawMessage(`{"query":"ok"}`))
	assertErrorCode(t, err, "home_unreachable")

	close(home.done)
	_, err = Tools["kb_search"].Call(t.Context(), h, caller, json.RawMessage(`{"query":"ok"}`))
	assertErrorCode(t, err, "home_unreachable")
}
