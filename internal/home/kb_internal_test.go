package home

import (
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestKBNoteTypeWriterCheck(t *testing.T) {
	noteTypes["human_only"] = func(w Who) *Refusal {
		if w.AI != "" {
			return &Refusal{"forbidden", "only a person writes this note type"}
		}
		return nil
	}
	defer delete(noteTypes, "human_only")

	refusal := kbValidTypedNote(proto.Note{NoteType: "human_only", Name: "Doc"}, Who{AI: "ai-1"})
	if refusal == nil || refusal.Code != "forbidden" {
		t.Fatalf("got %+v; want forbidden", refusal)
	}
	if refusal := kbValidTypedNote(proto.Note{NoteType: "human_only", Name: "Doc"}, Who{Human: "m-1"}); refusal != nil {
		t.Fatalf("unexpected refusal for person: %+v", refusal)
	}
}
