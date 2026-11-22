package home

import "github.com/huanchong-99/SoloDawn/internal/proto"

// Project memory: note writes a context note, and forget replaces a note or a
// review message with a gone marker (planning/contracts/commands.md 1.6;
// planning/contracts/rows.md sections 5 and 6).

func init() {
	Commands.Add("note", notePut)
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
