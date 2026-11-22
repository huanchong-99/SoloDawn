package home

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Knowledge base and typed notes: note with type and name, and fetch with
// kind kb and q (planning/contracts/step-3/knowledge.md sections 1, 2 and 3).

const kbNameMax = 80

// NamedNoteDisk is implemented by a Disk that reads a live typed note by its
// type and name inside the command's transaction.
type NamedNoteDisk interface {
	NamedNote(noteType, name string) (proto.NoteRow, bool)
}

var noteTypes = map[string]func(Who) *Refusal{
	"kb": kbAllowWriter,
}

func kbAllowWriter(Who) *Refusal { return nil }

func kbNotePut(s *State, c Command, p proto.Note) (Result, *Refusal) {
	if refusal := kbValidTypedNote(p, c.Who); refusal != nil {
		return Result{}, refusal
	}
	existing, replaced := kbLookupNamed(c.Disk, p.NoteType, p.Name)
	if !replaced {
		if _, taken := c.Disk.Note(string(p.ID)); taken {
			return Result{}, &Refusal{"conflict", "the note id is already in use"}
		}
	}
	row := kbMakeRow(s.ID, c, p, existing, replaced)
	return Result{
		Rows:   []Row{RowOf(row)},
		Answer: store.Outcome{ID: row.ID, Replaced: &replaced},
	}, nil
}

func kbValidTypedNote(p proto.Note, w Who) *Refusal {
	allow := noteTypes[p.NoteType]
	if allow == nil || p.Task != nil || !kbValidName(p.Name) {
		return &Refusal{"bad_request", "a typed note requires a known type, a valid name and null task"}
	}
	return allow(w)
}

func kbValidName(name string) bool {
	n := utf8.RuneCountInString(name)
	return 1 <= n && n <= kbNameMax && !strings.ContainsFunc(name, unicode.IsControl)
}

func kbLookupNamed(d Disk, noteType, name string) (proto.NoteRow, bool) {
	if nd, ok := d.(NamedNoteDisk); ok {
		return nd.NamedNote(noteType, name)
	}
	return proto.NoteRow{}, false
}

func kbMakeRow(group string, c Command, p proto.Note, existing proto.NoteRow, replaced bool) proto.NoteRow {
	id, pos := string(p.ID), int64(0)
	if replaced {
		id, pos = existing.ID, existing.Pos
	}
	return proto.NoteRow{
		RowHeader: proto.RowHeader{Type: noteRowType, ID: id},
		Pos:       pos,
		Group:     group,
		NoteType:  p.NoteType,
		Name:      p.Name,
		Text:      p.Text,
		By:        c.Who.Member(),
		Timestamp: c.Timestamp,
	}
}

func kbSearchOK(req Fetch) bool {
	return req.Kind == "kb" && req.Before == 0 && fetPageOK(req) && strings.TrimSpace(*req.Query) != ""
}
