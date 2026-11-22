package host

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func presetNoteRow(id string, pos int64, name, text string) proto.NoteRow {
	return proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: id, Seq: pos},
		Pos:       pos, Group: tGroupID, NoteType: "preset", Name: name, Text: text, By: tOwnerM, Timestamp: seamStart,
	}
}

func TestTurnPresetFirstAssignReviewAndReplaced(t *testing.T) {
	r := newTurnRig(t)
	r.brief.Preset = "Calm dashboard"
	presetText := "Principles: one accent colour;\nno motion longer than 200 ms."
	// Context note fills the 8 KiB budget while a second context note is trimmed; preset stays whole outside it.
	bigNote := strings.Repeat("n", maxNoteBytes)
	r.notes = []proto.NoteRow{
		{RowHeader: proto.RowHeader{Type: "note", ID: turnID("note-big"), Seq: 22}, Pos: 22, Group: tGroupID, Text: bigNote, By: tSecID, Timestamp: seamStart},
		{RowHeader: proto.RowHeader{Type: "note", ID: turnID("note-trim"), Seq: 21}, Pos: 21, Group: tGroupID, Text: "trimmed", By: tSecID, Timestamp: seamStart},
	}

	page1 := proto.Fetched{
		Type: "fetched", Success: proto.Success{Re: 2, OK: true}, More: true,
		Rows: []json.RawMessage{mustJSON(t, presetNoteRow(turnID("preset-other"), 30, "Other style", "other"))},
	}
	page2 := proto.Fetched{
		Type: "fetched", Success: proto.Success{Re: 3, OK: true}, More: false,
		Rows: []json.RawMessage{mustJSON(t, presetNoteRow(turnID("preset-calm"), 25, "Calm dashboard", presetText))},
	}

	r.home = newRoutedHome(func(req any) (json.RawMessage, error) {
		if fk, ok := req.(proto.FetchKind); ok && fk.Kind == "preset" {
			if fk.Before == 0 {
				return mustJSON(t, page1), nil
			}
			return mustJSON(t, page2), nil
		}
		return r.defaultHomeHandler(req)
	})
	r.h.Attach(r.homeKey.ID(), r.home)

	assignRaw := r.makeInboxItem(proto.Item{Item: 1, Why: "assign", Task: proto.ID(tTaskID), Msg: proto.ID(tTaskID)}, nil, nil)
	gotAssign, _, err := Compose(r.h, r.session(TaskSession, false, ""), ItemsTurn, []json.RawMessage{assignRaw})
	if err != nil {
		t.Fatal(err)
	}
	wantSection := "Reviewer: Codex   Pass score: 90\nStyle preset \"Calm dashboard\":\n" + presetText + "\nNotes of this group:\n- " + bigNote
	if !strings.Contains(gotAssign, wantSection) || strings.Contains(gotAssign, "trimmed") {
		t.Fatalf("assign turn mismatch:\n%s", gotAssign)
	}

	// Non-first assign turn (CLISession set) does not include the preset.
	gotLater, _, err := Compose(r.h, r.session(TaskSession, false, "cli-1"), ItemsTurn, []json.RawMessage{assignRaw})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gotLater, "Style preset") {
		t.Fatalf("later assign turn unexpectedly carried preset:\n%s", gotLater)
	}

	// Review turn carries only the preset name after Pass score, without fetching presets.
	r.home.asked = nil
	revRaw := r.makeInboxItem(proto.Item{Item: 2, AI: proto.ID(tPeerID), Why: "review", Task: proto.ID(tTaskID), Commit: "9f2c1ab"}, nil, nil)
	gotRev, _, err := Compose(r.h, r.session(ReviewSession, false, ""), ItemsTurn, []json.RawMessage{revRaw})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotRev, "Pass score: 90\nStyle preset: \"Calm dashboard\"") || len(r.home.asked) != 1 {
		t.Fatalf("review turn mismatch (asked=%d):\n%s", len(r.home.asked), gotRev)
	}

	// ReplacedTurn on TaskSession and ReviewSession preserves the respective preset rendering.
	gotRepTask, _, err := Compose(r.h, r.session(TaskSession, false, ""), ReplacedTurn, nil)
	if err != nil || !strings.Contains(gotRepTask, "Style preset \"Calm dashboard\":\n"+presetText) {
		t.Fatalf("replaced task turn err=%v:\n%s", err, gotRepTask)
	}
	gotRepRev, _, err := Compose(r.h, r.session(ReviewSession, false, ""), ReplacedTurn, nil)
	if err != nil || !strings.Contains(gotRepRev, "Pass score: 90\nStyle preset: \"Calm dashboard\"") {
		t.Fatalf("replaced review turn err=%v:\n%s", err, gotRepRev)
	}
}

func TestTurnPresetDeletedAndErrors(t *testing.T) {
	r := newTurnRig(t)
	r.brief.Preset = "Calm dashboard"
	assignRaw := r.makeInboxItem(proto.Item{Item: 1, Why: "assign", Task: proto.ID(tTaskID), Msg: proto.ID(tTaskID)}, nil, nil)

	// Deleted preset (empty page, even with More=true) inserts the deleted notice and keeps notes.
	r.home = newRoutedHome(func(req any) (json.RawMessage, error) {
		if fk, ok := req.(proto.FetchKind); ok && fk.Kind == "preset" {
			return mustJSON(t, proto.Fetched{Type: "fetched", Success: proto.Success{Re: 2, OK: true}, Rows: []json.RawMessage{}, More: true}), nil
		}
		return r.defaultHomeHandler(req)
	})
	r.h.Attach(r.homeKey.ID(), r.home)

	gotDeleted, _, err := Compose(r.h, r.session(TaskSession, false, ""), ItemsTurn, []json.RawMessage{assignRaw})
	if err != nil {
		t.Fatal(err)
	}
	wantDeleted := "Reviewer: Codex   Pass score: 90\nStyle preset \"Calm dashboard\": deleted since the brief was proposed.\nNotes of this group:"
	if !strings.Contains(gotDeleted, wantDeleted) {
		t.Fatalf("deleted preset turn mismatch:\n%s", gotDeleted)
	}

	// Invalid preset row surfaces bad_request.
	r.home = newRoutedHome(func(req any) (json.RawMessage, error) {
		if fk, ok := req.(proto.FetchKind); ok && fk.Kind == "preset" {
			return mustJSON(t, proto.Fetched{Type: "fetched", Success: proto.Success{Re: 2, OK: true}, Rows: []json.RawMessage{json.RawMessage(`{"r":"note"}`)}}), nil
		}
		return r.defaultHomeHandler(req)
	})
	r.h.Attach(r.homeKey.ID(), r.home)
	_, _, err = Compose(r.h, r.session(TaskSession, false, ""), ItemsTurn, []json.RawMessage{assignRaw})
	seamCode(t, err, "bad_request")

	// Home transport error during preset fetch surfaces home_unreachable.
	r.home = newRoutedHome(func(req any) (json.RawMessage, error) {
		if fk, ok := req.(proto.FetchKind); ok && fk.Kind == "preset" {
			return nil, io.EOF
		}
		return r.defaultHomeHandler(req)
	})
	r.h.Attach(r.homeKey.ID(), r.home)
	_, _, err = Compose(r.h, r.session(TaskSession, false, ""), ItemsTurn, []json.RawMessage{assignRaw})
	seamCode(t, err, "home_unreachable")

	// Session instructions include preset guidance on task and secretary sessions.
	checkInstructionCase(t, Session{Kind: TaskSession}, []string{"style preset"})
	checkInstructionCase(t, Session{Kind: GroupSession, Secretary: true}, []string{"style preset", "type: preset"})
}
