//go:build acceptance

package features

import (
	"bytes"
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
	var decision *proto.CardState
	save := func(r proto.MsgRow) error {
		return s.Write(func(tx *store.Tx) error {
			seq, err := tx.NextSeq(r.Group)
			if err != nil {
				return err
			}
			r.Seq = seq
			return store.Put(tx, r.Group, card.Timestamp+seq, r)
		})
	}
	sc.Step(`^the main line has a pending card followed by a message$`, func() error {
		var err error
		s, err = store.Open(":memory:")
		if err != nil {
			return err
		}
		if err := readWireExample("row-msg", &card); err != nil {
			return err
		}
		decision, card.State = card.State, &proto.CardState{Status: "pending"}
		card.Pos = 1
		if err := save(card); err != nil {
			return err
		}
		return save(proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: "later"},
			Group: card.Group, By: "system", Kind: "system", Main: true, Pos: 2,
			Timestamp: card.Timestamp + 1, Mentions: []string{}, Body: json.RawMessage(`{"code":"group_resumed"}`)})
	})
	sc.Step(`^the card is decided$`, func() error {
		card.State = decision
		return save(card)
	})
	sc.Step(`^syncing after sequence (\d+) returns only the decided card at sequence (\d+)$`, func(after, want int64) error {
		v, err := s.Snapshot(card.Group)
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
		return expectCard(rows, card, want, 1)
	})
	sc.Step(`^paging before position (\d+) returns that card at position (\d+)$`, func(before, want int64) error {
		v, err := s.Snapshot(card.Group)
		if err != nil {
			return err
		}
		defer v.Close()
		rows, err := v.PageThread(nil, before, 10)
		if err != nil {
			return err
		}
		return expectCard(rows, card, 3, want)
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if s != nil {
			return ctx, s.Close()
		}
		return ctx, nil
	})
}

func expectCard(rows []json.RawMessage, want proto.MsgRow, seq, pos int64) error {
	if len(rows) != 1 {
		return fmt.Errorf("want just the card, got %s", rows)
	}
	want.Seq, want.Pos = seq, pos
	data, err := json.Marshal(want)
	if err != nil {
		return err
	}
	if !bytes.Equal(rows[0], data) {
		return fmt.Errorf("got %s; want %s", rows[0], data)
	}
	return nil
}
