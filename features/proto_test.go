//go:build acceptance

package features

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func readWireExample(name string, v any) error {
	b, err := os.ReadFile(filepath.Join("..", "internal", "proto", "testdata", name+".json"))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func protocolSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the quota answer example reports (\d+) of (\d+) bytes$`, func(used, limit int64) error {
		b, err := os.ReadFile("../internal/proto/testdata/rv-quota-answer.json")
		if err != nil {
			return err
		}
		var q proto.Quota
		if err := proto.Decode(b, &q); err != nil {
			return err
		}
		if q != (proto.Quota{Type: "quota", Used: used, Limit: limit}) {
			return fmt.Errorf("unexpected quota: %+v", q)
		}
		return nil
	})
	approvalSteps(sc)
	var forwarded []byte
	sc.Step(`^a peer decodes and re-encodes the enrollment log$`, func() error {
		var log proto.Devlog
		if err := readWireExample("devlog", &log); err != nil {
			return err
		}
		var err error
		forwarded, err = json.Marshal(log)
		return err
	})
	sc.Step(`^both signatures verify for the original additions of "([^"]*)" and "([^"]*)"$`, func(first, second string) error {
		var log proto.Devlog
		if err := json.Unmarshal(forwarded, &log); err != nil {
			return err
		}
		pub, err := base64.RawURLEncoding.DecodeString(log.User)
		if err != nil {
			return err
		}
		if len(log.Entries) != 2 {
			return fmt.Errorf("expected two signed entries, got %+v", log)
		}
		for i, name := range []string{first, second} {
			env := log.Entries[i]
			sig, err := base64.RawURLEncoding.DecodeString(env.Signature)
			if err != nil {
				return err
			}
			if env.Key != log.User || !ed25519.Verify(pub, []byte("solodawn/1\n"+env.Payload), sig) {
				return fmt.Errorf("entry %d no longer verifies", i+1)
			}
			var entry proto.DeviceEntry
			if err := json.Unmarshal([]byte(env.Payload), &entry); err != nil {
				return err
			}
			if entry.User != log.User || entry.Version != i+1 || entry.Op != "add" || entry.Name != name {
				return fmt.Errorf("unexpected entry: %+v", entry)
			}
		}
		return nil
	})

	var update proto.Rows
	sc.Step(`^a client decodes the mixed group update$`, func() error {
		if err := readWireExample("rows", &update); err != nil {
			return err
		}
		if update.Type != "rows" || len(update.Rows) != 3 {
			return fmt.Errorf("unexpected batch: %+v", update)
		}
		return nil
	})
	sc.Step(`^it sees task (\d+) at step "([^"]*)"$`, func(num int, step string) error {
		var task proto.TaskRow
		if err := json.Unmarshal(update.Rows[0], &task); err != nil {
			return err
		}
		if task.Type != "task" || task.Group != update.Group || task.Num != num || task.Step != step {
			return fmt.Errorf("unexpected task: %+v", task)
		}
		return nil
	})
	sc.Step(`^the decision created at position (\d+) is now "([^"]*)" at sequence (\d+)$`, func(pos int64, status string, seq int64) error {
		var msg proto.MsgRow
		if err := json.Unmarshal(update.Rows[1], &msg); err != nil {
			return err
		}
		if msg.Kind != "decision" || msg.Pos != pos || msg.State == nil || msg.State.Status != status || msg.Seq != seq {
			return fmt.Errorf("unexpected decision: %+v", msg)
		}
		return nil
	})
	sc.Step(`^the note at sequence (\d+) is a deletion marker$`, func(seq int64) error {
		var gone proto.GoneRow
		if err := json.Unmarshal(update.Rows[2], &gone); err != nil {
			return err
		}
		if gone.Type != "note" || gone.Seq != seq || !gone.Gone {
			return fmt.Errorf("unexpected marker: %+v", gone)
		}
		return nil
	})
}

func approvalSteps(sc *godog.ScenarioContext) {
	var before proto.MessageRow[proto.ApprovalBody, proto.StatusOnly]
	var after proto.MessageRow[proto.ApprovalBody, proto.ActedState]
	sc.Step(`^a client reads the pending approval and its approved update$`, func() error {
		if err := readWireExample("row-msg-approval-pending", &before); err != nil {
			return err
		}
		return readWireExample("row-msg-approval-approved", &after)
	})
	sc.Step(`^both show the same signed request for "([^"]*)"$`, func(command string) error {
		if before.State.Status != "pending" || after.State.Status != "approved" || before.ID != after.ID ||
			before.Pos != after.Pos || before.Seq >= after.Seq || before.Body.Command != command ||
			!reflect.DeepEqual(before.Body, after.Body) || *before.Env != *after.Env {
			return fmt.Errorf("approval changed: before %+v; after %+v", before, after)
		}
		return nil
	})
	sc.Step(`^the update carries the approver's signed "([^"]*)" action for that card$`, func(action string) error {
		env := after.State.Act
		var act struct{ Msg, Action string }
		if err := json.Unmarshal([]byte(env.Payload), &act); err != nil {
			return err
		}
		known := func(key string) bool { return key == after.Body.Approvers[0] }
		header, err := identity.OpenCommand(env, "", known, time.UnixMilli(after.State.At))
		if err != nil {
			return err
		}
		if header.Type != "act" || string(header.Group) != after.Group || act.Msg != after.ID || act.Action != action {
			return fmt.Errorf("unexpected approval action: %+v", env)
		}
		return nil
	})
}
