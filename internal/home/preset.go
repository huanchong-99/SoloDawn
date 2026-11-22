package home

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Style presets: notes of type preset created or replaced by a person, and the
// preset check on post of kind brief (planning/contracts/step-3/knowledge.md
// sections 1, 2 and 4).

const (
	presetType      = "preset"
	presetNameLimit = 80
)

func presetNotePut(s *State, c Command, p proto.Note) (Result, *Refusal) {
	if refusal := presetCheckNote(c, p); refusal != nil {
		return Result{}, refusal
	}
	if old, ok := presetFind(c.Disk.TypedNotes(p.NoteType), p.Name); ok {
		return presetWrite(s, c, p, old.ID, old.Pos, true), nil
	}
	if _, taken := c.Disk.Note(string(p.ID)); taken {
		return Result{}, &Refusal{"conflict", "a note with that id already exists in the group"}
	}
	return presetWrite(s, c, p, string(p.ID), 0, false), nil
}

func presetCheckNote(c Command, p proto.Note) *Refusal {
	if !slices.Contains([]string{presetType, kbType}, p.NoteType) || !presetNameOK(p.Name) || p.Task != nil {
		return &Refusal{"bad_request", "a typed note requires a valid type, a name of 1 to 80 printable characters, and a null task"}
	}
	if p.NoteType == presetType && c.Who.AI != "" {
		return &Refusal{"forbidden", "only a person writes a style preset"}
	}
	return nil
}

func presetNameOK(name string) bool {
	n := utf8.RuneCountInString(name)
	return 1 <= n && n <= presetNameLimit && !strings.ContainsFunc(name, unicode.IsControl)
}

func presetWrite(s *State, c Command, p proto.Note, id string, pos int64, replaced bool) Result {
	row := proto.NoteRow{
		RowHeader: proto.RowHeader{Type: noteRowType, ID: id}, Pos: pos, Group: s.ID,
		NoteType: p.NoteType, Name: p.Name, Text: p.Text, By: c.Who.Member(), Timestamp: c.Timestamp,
	}
	return Result{
		Rows:   []Row{RowOf(row)},
		Answer: store.Outcome{ID: id, Replaced: &replaced},
	}
}

func presetBriefCheck(d Disk, preset string) *Refusal {
	if preset == "" {
		return nil
	}
	notes := d.TypedNotes(presetType)
	if _, ok := presetFind(notes, preset); ok {
		return nil
	}
	names := make([]string, len(notes))
	for i, n := range notes {
		names[i] = n.Name
	}
	return &Refusal{"not_found", presetUnknownMsg(preset, names)}
}

func presetFind(notes []proto.NoteRow, name string) (proto.NoteRow, bool) {
	i := slices.IndexFunc(notes, func(n proto.NoteRow) bool { return n.Name == name })
	if i < 0 {
		return proto.NoteRow{}, false
	}
	return notes[i], true
}

func presetUnknownMsg(given string, valid []string) string {
	names := "none"
	if len(valid) > 0 {
		names = strings.Join(valid, ", ")
	}
	return fmt.Sprintf("unknown preset %q (valid: %s)", given, names)
}
