package home_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

var (
	noteWorker = id(100)
	noteAdmin  = id(101)
	noteMember = id(102)
	noteTask   = id(103)
)

const noteTS = clock - 600

type noteDisk struct {
	msgs  map[string]proto.MsgRow
	notes map[string]proto.NoteRow
}

func (d noteDisk) Msg(id string) (proto.MsgRow, bool)        { m, ok := d.msgs[id]; return m, ok }
func (d noteDisk) Note(id string) (proto.NoteRow, bool)      { n, ok := d.notes[id]; return n, ok }
func (d noteDisk) TypedNotes(string) []proto.NoteRow         { return nil }
func (d noteDisk) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (d noteDisk) Pending() []proto.MsgRow                   { return nil }
func (d noteDisk) Queued() []store.Item                      { return nil }
func (d noteDisk) OtherMember(string) bool                   { return false }
func (noteDisk) DeviceLog(string) []proto.Envelope           { return nil }

func noteState(t *testing.T) *home.State {
	t.Helper()
	s := group(t)
	second := device(15)
	s.Logs[second.ID()] = logOf(t, second)
	s.People[noteAdmin] = person(noteAdmin, second, "admin")
	s.People[noteMember] = person(noteMember, guest, "member")
	s.AIs[noteWorker] = ai(noteWorker, laptop)
	s.Tasks[noteTask] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: noteTask, Seq: 9}, State: "done"}
	return s
}

func noteApply(t *testing.T, s *home.State, d noteDisk, r home.Receipts, by identity.Key, payload string) home.Result {
	t.Helper()
	res, err := home.Commands.Apply(s, d, r, by.Sign(payload), now)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func noteNulls(t *testing.T, v any, allowed ...string) {
	t.Helper()
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		t.Fatal(err)
	}
	equal(t, proto.UnexpectedNulls(bytes.TrimSuffix(b.Bytes(), []byte("\n")), allowed...), []string{})
}

func noteRefused(t *testing.T, res home.Result, code string) {
	t.Helper()
	if res.Answer.Code != code || res.Msg == "" || len(res.Rows) != 0 || len(res.Items) != 0 {
		t.Fatalf("answered %+v (%q) with %d rows and %d items; want %s", res.Answer, res.Msg, len(res.Rows), len(res.Items), code)
	}
}

func TestNotePutWritesNoteRow(t *testing.T) {
	s := noteState(t)
	d := noteDisk{msgs: map[string]proto.MsgRow{}, notes: map[string]proto.NoteRow{}}
	first := fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"text":"Keep <paths> clean."}`, id(110), noteTS, groupID, id(111))
	res := noteApply(t, s, d, receipts{}, browser, first)
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, res.Items, ([]home.Item)(nil))
	want := proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: id(111), Seq: 11}, Pos: 11, Group: groupID,
		Text: "Keep <paths> clean.", By: ownerID, Timestamp: noteTS,
	}
	equal(t, []any{res.Rows[0].Value()}, []any{want})
	noteNulls(t, res.Rows[0].Value(), "/task")
	s.Commit(res)

	second := fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"as":%q,"id":%q,"task":%q,"text":""}`, id(112), noteTS, groupID, noteWorker, id(113), noteTask)
	res = noteApply(t, s, d, receipts{}, laptop, second)
	equal(t, res.Answer, store.Outcome{OK: true})
	taskRef := noteTask
	wantTask := proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: id(113), Seq: 12}, Pos: 12, Group: groupID,
		Task: &taskRef, Text: "", By: noteWorker, Timestamp: noteTS,
	}
	equal(t, []any{res.Rows[0].Value()}, []any{wantTask})
	noteNulls(t, res.Rows[0].Value())
}

func TestNotePutRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		payload string
		code    string
	}{
		"non-id note id": {
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":"bad","task":null,"text":"x"}`, id(114), noteTS, groupID),
			code:    "bad_request",
		},
		"null text": {
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"text":null}`, id(114), noteTS, groupID, id(115)),
			code:    "bad_request",
		},
		"non-id task": {
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":"bad","text":"x"}`, id(114), noteTS, groupID, id(115)),
			code:    "bad_request",
		},
		"unknown task": {
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":%q,"text":"x"}`, id(114), noteTS, groupID, id(115), id(119)),
			code:    "not_found",
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := noteState(t)
			d := noteDisk{msgs: map[string]proto.MsgRow{}, notes: map[string]proto.NoteRow{}}
			noteRefused(t, noteApply(t, s, d, receipts{}, browser, tc.payload), tc.code)
		})
	}
}

func TestNoteForgetReplacesNoteAndReviewWithGoneMarker(t *testing.T) {
	s := noteState(t)
	d := noteDisk{
		msgs:  map[string]proto.MsgRow{id(120): {RowHeader: proto.RowHeader{Type: "msg", ID: id(120)}, Kind: "review"}},
		notes: map[string]proto.NoteRow{id(121): {RowHeader: proto.RowHeader{Type: "note", ID: id(121)}}},
	}
	r := receipts{}
	cmd := fmt.Sprintf(`{"t":"forget","n":%q,"ts":%d,"g":%q,"r":"note","id":%q}`, id(122), noteTS, groupID, id(121))
	res := noteApply(t, s, d, r, browser, cmd)
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, []any{res.Rows[0].Value()}, []any{proto.GoneRow{RowHeader: proto.RowHeader{Type: "note", ID: id(121), Seq: 11}, Gone: true}})
	noteNulls(t, res.Rows[0].Value())
	s.Commit(res)
	delete(d.notes, id(121))

	again := noteApply(t, s, d, r, browser, cmd)
	equal(t, again, home.Result{Answer: store.Outcome{OK: true}})

	adminDev := device(15)
	msgCmd := fmt.Sprintf(`{"t":"forget","n":%q,"ts":%d,"g":%q,"r":"msg","id":%q}`, id(123), noteTS, groupID, id(120))
	res = noteApply(t, s, d, r, adminDev, msgCmd)
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, []any{res.Rows[0].Value()}, []any{proto.GoneRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(120), Seq: 12}, Gone: true}})
	noteNulls(t, res.Rows[0].Value())
	s.Commit(res)
	delete(d.msgs, id(120))

	fresh := fmt.Sprintf(`{"t":"forget","n":%q,"ts":%d,"g":%q,"r":"msg","id":%q}`, id(124), noteTS, groupID, id(120))
	noteRefused(t, noteApply(t, s, d, r, browser, fresh), "not_found")
}

func TestNoteForgetRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		by      identity.Key
		payload string
		code    string
	}{
		"by a non-admin member": {
			by:      guest,
			payload: fmt.Sprintf(`{"t":"forget","n":%q,"ts":%d,"g":%q,"r":"note","id":%q}`, id(130), noteTS, groupID, id(121)),
			code:    "forbidden",
		},
		"by the secretary AI": {
			by:      laptop,
			payload: fmt.Sprintf(`{"t":"forget","n":%q,"ts":%d,"g":%q,"as":%q,"r":"note","id":%q}`, id(130), noteTS, groupID, aiID, id(121)),
			code:    "forbidden",
		},
		"by a working AI": {
			by:      laptop,
			payload: fmt.Sprintf(`{"t":"forget","n":%q,"ts":%d,"g":%q,"as":%q,"r":"note","id":%q}`, id(130), noteTS, groupID, noteWorker, id(121)),
			code:    "forbidden",
		},
		"with a bad target id": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"forget","n":%q,"ts":%d,"g":%q,"r":"note","id":"bad"}`, id(130), noteTS, groupID),
			code:    "bad_request",
		},
		"for a task row": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"forget","n":%q,"ts":%d,"g":%q,"r":"task","id":%q}`, id(130), noteTS, groupID, noteTask),
			code:    "bad_request",
		},
		"for an empty row type": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"forget","n":%q,"ts":%d,"g":%q,"r":"","id":%q}`, id(130), noteTS, groupID, id(121)),
			code:    "bad_request",
		},
		"for a missing note": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"forget","n":%q,"ts":%d,"g":%q,"r":"note","id":%q}`, id(130), noteTS, groupID, id(135)),
			code:    "not_found",
		},
		"for a missing message": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"forget","n":%q,"ts":%d,"g":%q,"r":"msg","id":%q}`, id(130), noteTS, groupID, id(135)),
			code:    "not_found",
		},
		"for a text message": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"forget","n":%q,"ts":%d,"g":%q,"r":"msg","id":%q}`, id(130), noteTS, groupID, id(131)),
			code:    "conflict",
		},
		"for a brief card": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"forget","n":%q,"ts":%d,"g":%q,"r":"msg","id":%q}`, id(130), noteTS, groupID, id(132)),
			code:    "conflict",
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := noteState(t)
			d := noteDisk{
				msgs: map[string]proto.MsgRow{
					id(131): {RowHeader: proto.RowHeader{Type: "msg", ID: id(131)}, Kind: "text"},
					id(132): {RowHeader: proto.RowHeader{Type: "msg", ID: id(132)}, Kind: "brief"},
				},
				notes: map[string]proto.NoteRow{
					id(121): {RowHeader: proto.RowHeader{Type: "note", ID: id(121)}},
				},
			}
			noteRefused(t, noteApply(t, s, d, receipts{}, tc.by, tc.payload), tc.code)
		})
	}
}

func noteEditDisk() *presetDisk {
	d := presetNewDisk()
	task := noteTask
	d.notes[id(140)] = proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: id(140), Seq: 7}, Pos: 7, Group: groupID,
		Task: &task, Text: "Old lesson.", By: noteWorker, Timestamp: noteTS - 100,
	}
	for i, name := range []string{"Decision A", "Decision B"} {
		d.notes[id(byte(141+i))] = proto.NoteRow{
			RowHeader: proto.RowHeader{Type: "note", ID: id(byte(141 + i)), Seq: int64(8 + i)}, Pos: int64(8 + i), Group: groupID,
			NoteType: "kb", Name: name, Text: "Why.", By: noteWorker, Timestamp: noteTS - 100,
		}
	}
	return d
}

func noteEditCmd(n byte, note, name, text string) string {
	named := ""
	if name != "" {
		named = fmt.Sprintf(`,"name":%q`, name)
	}
	return fmt.Sprintf(`{"t":"note_edit","n":%q,"ts":%d,"g":%q,"id":%q%s,"text":%q}`, id(n), noteTS, groupID, note, named, text)
}

func TestNoteEditChangesTextAndNameInPlace(t *testing.T) {
	s := noteState(t)
	d := noteEditDisk()
	res := presetApply(t, s, d, receipts{}, guest, noteEditCmd(150, id(140), "", "New lesson."))
	equal(t, res.Answer, store.Outcome{OK: true})
	task := noteTask
	equal(t, []any{res.Rows[0].Value()}, []any{proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: id(140), Seq: 11}, Pos: 7, Group: groupID,
		Task: &task, Text: "New lesson.", By: noteMember, Timestamp: noteTS,
	}})
	s.Commit(res)

	for i, name := range []string{"Decision C", "Decision C"} {
		res = presetApply(t, s, d, receipts{}, browser, noteEditCmd(byte(151+i), id(141), name, "Because."))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, []any{res.Rows[0].Value()}, []any{proto.NoteRow{
			RowHeader: proto.RowHeader{Type: "note", ID: id(141), Seq: int64(12 + i)}, Pos: 8, Group: groupID,
			NoteType: "kb", Name: name, Text: "Because.", By: ownerID, Timestamp: noteTS,
		}})
		s.Commit(res)
		d.record(res)
	}
}

func TestNoteEditRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		by      identity.Key
		payload string
		code    string
	}{
		"signed for an AI": {
			by:      laptop,
			payload: strings.Replace(noteEditCmd(160, id(140), "", "x"), `"id"`, fmt.Sprintf(`"as":%q,"id"`, noteWorker), 1),
			code:    "forbidden",
		},
		"with a bad id":            {browser, noteEditCmd(160, "bad", "", "x"), "bad_request"},
		"of a missing note":        {browser, noteEditCmd(160, id(149), "", "x"), "not_found"},
		"naming a context note":    {browser, noteEditCmd(160, id(140), "Lesson", "x"), "bad_request"},
		"of a typed note unnamed":  {browser, noteEditCmd(160, id(141), "", "x"), "bad_request"},
		"to a name another has":    {browser, noteEditCmd(160, id(141), "Decision B", "x"), "conflict"},
		"to a name with a control": {browser, noteEditCmd(160, id(141), "a\tb", "x"), "bad_request"},
	} {
		t.Run(name, func(t *testing.T) {
			noteRefused(t, presetApply(t, noteState(t), noteEditDisk(), receipts{}, tc.by, tc.payload), tc.code)
		})
	}
}
