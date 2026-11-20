//go:build acceptance

package features

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cucumber/godog"
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
