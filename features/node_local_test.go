//go:build acceptance

package features

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["node_local"] = nodeLocalSteps
}

type scenarioTB struct {
	testing.TB
	cleanup func(func())
}

func (s scenarioTB) Cleanup(fn func()) { s.cleanup(fn) }

func (s scenarioTB) Setenv(key, value string) {
	old, had := os.LookupEnv(key)
	os.Setenv(key, value)
	s.cleanup(func() { restoreVar(key, old, had) })
}

func nodeLocalSteps(sc *godog.ScenarioContext, f fixtures) {
	cleanup := afterScenario(sc)
	var (
		cluster *nodetest.Cluster
		groups  map[string]string
		pushes  chan proto.Rows
		seq     int64
	)
	nextID := func() string {
		seq++
		var raw [16]byte
		binary.BigEndian.PutUint64(raw[8:], uint64(seq))
		return base64.RawURLEncoding.EncodeToString(raw[:])
	}
	tables := func() home.Tables {
		return home.Tables{
			Commands: home.Table{
				"group/create": func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
					var p struct {
						Name string `json:"name"`
					}
					if ref := c.Decode(&p); ref != nil {
						return home.Result{}, ref
					}
					grp := proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: p.Name, Checks: []string{}, Created: c.Now}
					mem := proto.HumanMemberRow{
						MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "m-owner"}, Group: s.ID, Kind: "human", Name: "Owner", User: s.Owner},
						Role:      "admin",
					}
					return home.Result{Rows: []home.Row{home.RowOf(grp), home.RowOf(mem)}, GroupsChanged: true}, nil
				},
				"note": func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
					var p struct {
						Text string `json:"text"`
					}
					if ref := c.Decode(&p); ref != nil {
						return home.Result{}, ref
					}
					row := proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: string(c.Nonce)}, Group: s.ID, Text: p.Text, By: "m-owner", Timestamp: c.Now}
					return home.Result{Rows: []home.Row{home.RowOf(row)}}, nil
				},
			},
		}
	}

	sc.Step(`^an in-process cluster of (\d+) nodes?$`, func(n int) {
		groups = map[string]string{}
		// Single reader: the "receives a rows push" step in this scenario.
		pushes = make(chan proto.Rows, 8)
		host.Pushes["rows"] = func(_ *host.Host, _ string, raw json.RawMessage) {
			var frame proto.Rows
			if proto.Decode(raw, &frame) == nil {
				pushes <- frame
			}
		}
		cleanup(func() { delete(host.Pushes, "rows") })
		cluster = nodetest.Start(scenarioTB{TB: f.t, cleanup: cleanup}, nodetest.Config{
			BinDir: f.fakeCLIs,
			Nodes:  n,
			Tables: tables(),
		})
	})

	sc.Step(`^the node's host creates group "([^"]+)" on its local home$`, func(name string) error {
		n0 := cluster.Nodes[0]
		gid := nextID()
		groups[name] = gid
		payload := fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q,"name":%q}`, nextID(), n0.Host.Now().UnixMilli(), gid, name)
		_, err := n0.Host.Command(context.Background(), n0.Key.ID(), payload)
		return err
	})

	sc.Step(`^the node's local link watches group "([^"]+)"$`, func(name string) error {
		n0 := cluster.Nodes[0]
		if _, ok := n0.Link.Watch(groups[name], false); !ok {
			return fmt.Errorf("Watch(%q) refused", name)
		}
		return nil
	})

	sc.Step(`^the node's host writes note "([^"]+)" to group "([^"]+)"$`, func(text, name string) error {
		n0 := cluster.Nodes[0]
		payload := fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"text":%q}`, nextID(), n0.Host.Now().UnixMilli(), groups[name], text)
		_, err := n0.Host.Command(context.Background(), n0.Key.ID(), payload)
		return err
	})

	sc.Step(`^the node's host receives a rows push for group "([^"]+)" containing "([^"]+)"$`, func(name, text string) error {
		select {
		case frame := <-pushes:
			if frame.Group != groups[name] || len(frame.Rows) != 1 || !strings.Contains(string(frame.Rows[0]), text) {
				return fmt.Errorf("got push %+v, want group %q containing %q", frame, groups[name], text)
			}
			return nil
		case <-time.After(5 * time.Second):
			return fmt.Errorf("timed out waiting for rows push for %q", name)
		}
	})

	sc.Step(`^the node's local link reports path "([^"]+)"$`, func(want string) error {
		if got := cluster.Nodes[0].Link.Path(); got != want {
			return fmt.Errorf("Path() = %q, want %q", got, want)
		}
		return nil
	})
}
