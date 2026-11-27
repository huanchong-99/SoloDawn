package home

import "github.com/huanchong-99/SoloDawn/internal/proto"

// Project memory: note writes a context note, note_edit changes a note in
// place, and forget replaces a note or a review message with a gone marker
// (planning/contracts/commands.md 1.6; planning/contracts/rows.md sections 5
// and 6).

func init() {
	Commands.Add("note", notePut)
	Commands.Add("note_edit", noteEdit)
	Commands.Add("forget", noteForget)
}

const (
	noteRowType   = "note"
	noteMsgType   = "msg"
	noteReviewMsg = "review"
)

func notePut(s *State, c Command) (Result, *Refusal) {
	var p proto.Note
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if p.NoteType != "" || p.Name != "" {
		return presetNotePut(s, c, p)
	}
	if p.Task != nil {
		if _, ok := s.Tasks[string(*p.Task)]; !ok {
			return Result{}, &Refusal{"not_found", "the note names no task"}
		}
	}
	row := proto.NoteRow{
		RowHeader: proto.RowHeader{Type: noteRowType, ID: string(p.ID)}, Group: s.ID,
		Task: (*string)(p.Task), Text: p.Text, By: c.Who.Member(), Timestamp: c.Timestamp,
	}
	return Result{Rows: []Row{RowOf(row)}}, nil
}

func noteEdit(_ *State, c Command) (Result, *Refusal) {
	if c.Who.AI != "" {
		return Result{}, &Refusal{"forbidden", "only a person edits a note"}
	}
	var p proto.NoteEdit
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	note, ok := c.Disk.Note(string(p.ID))
	if !ok {
		return Result{}, &Refusal{"not_found", "the note does not exist"}
	}
	if refusal := noteRename(c.Disk, note, p.Name); refusal != nil {
		return Result{}, refusal
	}
	note.Name, note.Text, note.By, note.Timestamp = p.Name, p.Text, c.Who.Member(), c.Timestamp
	return Result{Rows: []Row{RowOf(note)}}, nil
}

// noteRename checks the name of an edit: none on a context note, and on a
// typed note one that no other note of its type has.
func noteRename(d Disk, note proto.NoteRow, name string) *Refusal {
	if note.NoteType == "" {
		if name != "" {
			return &Refusal{"bad_request", "a context note has no name"}
		}
		return nil
	}
	if !presetNameOK(name) {
		return &Refusal{"bad_request", "a typed note requires a name of 1 to 80 printable characters"}
	}
	if other, taken := presetFind(d.TypedNotes(note.NoteType), name); taken && other.ID != note.ID {
		return &Refusal{"conflict", "another note of that type has that name"}
	}
	return nil
}

func noteForget(s *State, c Command) (Result, *Refusal) {
	if !s.Admin(c.Who) {
		return Result{}, &Refusal{"forbidden", "only an admin deletes from the project memory"}
	}
	var p proto.Forget
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if refusal := noteForgettable(c.Disk, p.Row, string(p.ID)); refusal != nil {
		return Result{}, refusal
	}
	gone := proto.GoneRow{RowHeader: proto.RowHeader{Type: p.Row, ID: string(p.ID)}, Gone: true}
	return Result{Rows: []Row{RowOf(gone)}}, nil
}

func noteForgettable(d Disk, r, id string) *Refusal {
	switch r {
	case noteRowType:
		if _, ok := d.Note(id); !ok {
			return &Refusal{"not_found", "the note does not exist"}
		}
		return nil
	case noteMsgType:
		return noteForgetMsg(d, id)
	default:
		return &Refusal{"bad_request", "forget applies to a note or a message"}
	}
}

func noteForgetMsg(d Disk, id string) *Refusal {
	msg, ok := d.Msg(id)
	if !ok {
		return &Refusal{"not_found", "the message does not exist"}
	}
	if msg.Kind != noteReviewMsg {
		return &Refusal{"conflict", "only a review message can be forgotten"}
	}
	return nil
}
