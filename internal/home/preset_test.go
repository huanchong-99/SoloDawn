package home_test

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type presetDisk struct {
	msgs  map[string]proto.MsgRow
	notes map[string]proto.NoteRow
}

func presetNewDisk() *presetDisk {
	return &presetDisk{msgs: map[string]proto.MsgRow{}, notes: map[string]proto.NoteRow{}}
}

func (d *presetDisk) Msg(id string) (proto.MsgRow, bool)   { m, ok := d.msgs[id]; return m, ok }
func (d *presetDisk) Note(id string) (proto.NoteRow, bool) { n, ok := d.notes[id]; return n, ok }
func (d *presetDisk) Thread(*string, int64, int) []proto.MsgRow {
	return nil
}
func (d *presetDisk) Pending() []proto.MsgRow { return nil }
func (d *presetDisk) Queued() []store.Item    { return nil }

func (d *presetDisk) TypedNotes(noteType string) []proto.NoteRow {
	notes := []proto.NoteRow{}
	for _, n := range d.notes {
		if n.NoteType == noteType {
			notes = append(notes, n)
		}
	}
	slices.SortFunc(notes, func(a, b proto.NoteRow) int { return cmp.Compare(a.Name, b.Name) })
	return notes
}

func (d *presetDisk) record(res home.Result) {
	for _, r := range res.Rows {
		switch v := r.Value().(type) {
		case proto.NoteRow:
			d.notes[v.ID] = v
		case proto.MsgRow:
			d.msgs[v.ID] = v
		case proto.GoneRow:
			delete(d.notes, v.ID)
			delete(d.msgs, v.ID)
		}
	}
}

func presetApply(t *testing.T, s *home.State, d *presetDisk, r home.Receipts, by identity.Key, payload string) home.Result {
	t.Helper()
	res, err := home.Commands.Apply(s, d, r, by.Sign(payload), now)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestPresetNoteCreatesReplacesAndReplays(t *testing.T) {
	s := noteState(t)
	d := presetNewDisk()
	r := receipts{}

	createID := id(141)
	createCmd := fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"preset","name":"Calm dashboard","text":"v1 <accent>"}`, id(140), noteTS, groupID, createID)
	res := presetApply(t, s, d, r, browser, createCmd)
	f, tr := false, true
	equal(t, res.Answer, store.Outcome{OK: true, ID: createID, Replaced: &f})
	equal(t, res.Frame(9), proto.CmdResult{Success: proto.Success{Re: 9, OK: true}, ID: createID, Replaced: &f})
	wantV1 := proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: createID, Seq: 11}, Pos: 11, Group: groupID,
		NoteType: "preset", Name: "Calm dashboard", Text: "v1 <accent>", By: ownerID, Timestamp: noteTS,
	}
	equal(t, []any{res.Rows[0].Value()}, []any{wantV1})
	noteNulls(t, res.Rows[0].Value(), "/task")
	s.Commit(res)
	d.record(res)

	// Replaying the same command returns the stored id and replaced=false without advancing Seq.
	replay := presetApply(t, s, d, r, browser, createCmd)
	equal(t, replay, home.Result{Answer: store.Outcome{OK: true, ID: createID, Replaced: &f}})
	equal(t, replay.Frame(10), proto.CmdResult{Success: proto.Success{Re: 10, OK: true}, ID: createID, Replaced: &f})

	// Another person (non-admin member) replaces the preset; command's own id is ignored and id/pos stay.
	otherNoteID := id(142)
	d.notes[otherNoteID] = proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: otherNoteID, Seq: 5}, Pos: 5, Group: groupID, Text: "context"}
	replaceCmd := fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"preset","name":"Calm dashboard","text":"v2 updated"}`, id(143), noteTS+100, groupID, otherNoteID)
	res2 := presetApply(t, s, d, r, guest, replaceCmd)
	equal(t, res2.Answer, store.Outcome{OK: true, ID: createID, Replaced: &tr})
	equal(t, res2.Frame(11), proto.CmdResult{Success: proto.Success{Re: 11, OK: true}, ID: createID, Replaced: &tr})
	wantV2 := proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: createID, Seq: 12}, Pos: 11, Group: groupID,
		NoteType: "preset", Name: "Calm dashboard", Text: "v2 updated", By: noteMember, Timestamp: noteTS + 100,
	}
	equal(t, []any{res2.Rows[0].Value()}, []any{wantV2})
	s.Commit(res2)
	d.record(res2)

	// An 80-character Unicode name is accepted at the upper boundary.
	maxName := strings.Repeat("静", 80)
	maxCmd := fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"preset","name":%q,"text":"ok"}`, id(144), noteTS, groupID, id(145), maxName)
	res3 := presetApply(t, s, d, r, browser, maxCmd)
	equal(t, res3.Answer, store.Outcome{OK: true, ID: id(145), Replaced: &f})
}

func TestPresetNoteRefusals(t *testing.T) {
	takenID := id(150)
	for name, tc := range map[string]struct {
		by      identity.Key
		payload string
		code    string
		msgSub  string
	}{
		"type without name": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"preset","text":"x"}`, id(151), noteTS, groupID, id(152)),
			code:    "bad_request",
		},
		"name without type": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"name":"Calm","text":"x"}`, id(151), noteTS, groupID, id(152)),
			code:    "bad_request",
		},
		"empty name": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"preset","name":"","text":"x"}`, id(151), noteTS, groupID, id(152)),
			code:    "bad_request",
		},
		"unsupported type": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"other","name":"Calm","text":"x"}`, id(151), noteTS, groupID, id(152)),
			code:    "bad_request",
		},
		"name over 80 characters": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"preset","name":%q,"text":"x"}`, id(151), noteTS, groupID, id(152), strings.Repeat("a", 81)),
			code:    "bad_request",
		},
		"name with control character": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"preset","name":"Calm\nDashboard","text":"x"}`, id(151), noteTS, groupID, id(152)),
			code:    "bad_request",
		},
		"non-null task": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":%q,"type":"preset","name":"Calm","text":"x"}`, id(151), noteTS, groupID, id(152), noteTask),
			code:    "bad_request",
		},
		"signed for secretary AI": {
			by:      laptop,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"as":%q,"id":%q,"task":null,"type":"preset","name":"Calm","text":"x"}`, id(151), noteTS, groupID, aiID, id(152)),
			code:    "forbidden",
		},
		"signed for working AI": {
			by:      laptop,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"as":%q,"id":%q,"task":null,"type":"preset","name":"Calm","text":"x"}`, id(151), noteTS, groupID, noteWorker, id(152)),
			code:    "forbidden",
		},
		"duplicate note id on create": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"preset","name":"Calm","text":"x"}`, id(151), noteTS, groupID, takenID),
			code:    "conflict",
		},
		"row above 16 KiB limit": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"preset","name":"Calm","text":%q}`, id(151), noteTS, groupID, id(152), strings.Repeat("x", 16500)),
			code:    "bad_request",
			msgSub:  "above its limit of 16384",
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := noteState(t)
			d := presetNewDisk()
			d.notes[takenID] = proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: takenID}}
			res := presetApply(t, s, d, receipts{}, tc.by, tc.payload)
			noteRefused(t, res, tc.code)
			if tc.msgSub != "" && !strings.Contains(res.Msg, tc.msgSub) {
				t.Fatalf("msg %q missing %q", res.Msg, tc.msgSub)
			}
		})
	}
}

func TestPresetBriefPostAndConfirmAfterForget(t *testing.T) {
	s := briefState(t)
	d := presetNewDisk()
	r := receipts{}

	// Unknown preset when group has none lists "valid: none".
	missingBody := briefBody()
	missingBody.Preset = "Calm dashboard"
	missingEnv := briefSign(laptop, briefPost(160, missingBody))
	res, err := home.Commands.Apply(s, d, r, missingEnv, now)
	if err != nil {
		t.Fatal(err)
	}
	noteRefused(t, res, "not_found")
	if !strings.Contains(res.Msg, `unknown preset "Calm dashboard" (valid: none)`) {
		t.Fatalf("unexpected refusal msg: %q", res.Msg)
	}

	// Seed two presets; an unknown preset lists sorted valid names.
	d.notes[id(161)] = proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: id(161)}, NoteType: "preset", Name: "Calm dashboard"}
	d.notes[id(162)] = proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: id(162)}, NoteType: "preset", Name: "Airy"}
	missingEnv2 := briefSign(laptop, briefPost(163, missingBody))
	missingBody2 := briefBody()
	missingBody2.Preset = "Neon"
	missingEnv2 = briefSign(laptop, briefPost(163, missingBody2))
	res, err = home.Commands.Apply(s, d, r, missingEnv2, now)
	if err != nil {
		t.Fatal(err)
	}
	noteRefused(t, res, "not_found")
	if !strings.Contains(res.Msg, `unknown preset "Neon" (valid: Airy, Calm dashboard)`) {
		t.Fatalf("unexpected refusal msg: %q", res.Msg)
	}

	// Valid preset is stored on the brief card; confirming after the preset is forgotten still succeeds.
	validEnv := briefSign(laptop, briefPost(165, missingBody))
	resValid, err := home.Commands.Apply(s, d, r, validEnv, now)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, resValid.Answer, store.Outcome{OK: true})
	s.Commit(resValid)
	d.record(resValid)
	card := d.msgs[id(165)]
	var stored proto.BriefBody
	if err := json.Unmarshal(card.Body, &stored); err != nil {
		t.Fatal(err)
	}
	equal(t, stored.Preset, "Calm dashboard")

	delete(d.notes, id(161))
	confirmEnv := briefAct(browser, 166, "confirm", id(165), "")
	resConfirm, err := home.Commands.Apply(s, d, r, confirmEnv, now)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, resConfirm.Answer, store.Outcome{OK: true})
	equal(t, len(resConfirm.Rows), 5)
}
