//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func storeSteps(sc *godog.ScenarioContext) {
	var s *store.Store
	var card proto.MsgRow
	save := func(r proto.MsgRow) error {
		return s.Write(func(tx *store.Tx) error {
			seq, err := tx.NextSeq("team")
			if err != nil {
				return err
			}
			r.Seq = seq
			return store.Put(tx, "team", 100+seq, r)
		})
	}
	sc.Step(`^the main line has a pending card followed by a message$`, func() error {
		var err error
		s, err = store.Open(":memory:")
		if err != nil {
			return err
		}
		card = proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: "decision"}, Group: "team", Kind: "decision", Pos: 1, Main: true, Timestamp: 101, State: &proto.CardState{Status: "pending"}}
		if err := save(card); err != nil {
			return err
		}
		return save(proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: "later"}, Group: "team", Kind: "text", Pos: 2, Timestamp: 102, Text: "Later message"})
	})
	sc.Step(`^the card is decided$`, func() error {
		card.State = &proto.CardState{Status: "decided"}
		return save(card)
	})
	sc.Step(`^syncing after sequence (\d+) returns only the decided card at sequence (\d+)$`, func(after, want int64) error {
		v, err := s.Snapshot("team")
		if err != nil {
			return err
		}
		defer v.Close()
		if v.Seq != want {
			return fmt.Errorf("sync cut %d, want %d", v.Seq, want)
		}
		rows, err := v.AfterSeqUpTo(after, v.Seq)
		if err != nil {
			return err
		}
		return expectCard(rows, want, 1)
	})
	sc.Step(`^paging before position (\d+) returns that card at position (\d+)$`, func(before, want int64) error {
		v, err := s.Snapshot("team")
		if err != nil {
			return err
		}
		defer v.Close()
		rows, err := v.PageThread(nil, before, 10)
		if err != nil {
			return err
		}
		return expectCard(rows, 3, want)
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if s != nil {
			return ctx, s.Close()
		}
		return ctx, nil
	})
}

func expectCard(rows []json.RawMessage, seq, pos int64) error {
	if len(rows) != 1 {
		return fmt.Errorf("want just the card, got %s", rows)
	}
	var card proto.MsgRow
	if err := json.Unmarshal(rows[0], &card); err != nil {
		return err
	}
	if card.ID != "decision" || card.Seq != seq || card.Pos != pos || card.State == nil || card.State.Status != "decided" {
		return fmt.Errorf("unexpected card: %s", rows[0])
	}
	return nil
}
