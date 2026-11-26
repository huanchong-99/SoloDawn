//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["node_version"] = nodeVersionSteps
}

// oldHomeAnswers are the answers a home of v3.0.0-rc.1 may give to a hello of
// protocol version 3, by the scenario's words; %d is the hello's id.
var oldHomeAnswers = map[string]string{
	"the error version":   `{"re":%d,"err":{"code":"version","msg":"unsupported protocol version"}}`,
	"a version-2 welcome": `{"t":"welcome","re":%d,"ok":true,"v":2,"device":"d","name":"rc.1","version":"v3.0.0-rc.1","time":1,"addrs":[],"groups":[],"owner":false}`,
}

// nodeVersionWorld is a node of this version, run as the solodawn binary,
// and a fake home of rc.1 that it remembers, reached over a rendezvous.
type nodeVersionWorld struct {
	f       fixtures
	cleanup func(func())
	ctx     context.Context
	home    identity.Key
	answer  string
	data    string // the node's data directory
	output  syncBuffer

	mu    sync.Mutex
	conns [][]string // per connection from the node, the frames it sent, as "hello v<n>" or their t
}

func nodeVersionSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &nodeVersionWorld{f: f, cleanup: afterScenario(sc)}
	sc.Step(`^a node of this version that remembers a home which answers its hello with (the error version|a version-2 welcome)$`, w.start)
	sc.Step(`^the node has connected to that home twice$`, w.waitTwoConnections)
	sc.Step(`^each connection carried one hello, of protocol version 3, and nothing after it$`, w.checkHellos)
	sc.Step(`^the node's log says that the home speaks another protocol version$`, w.checkReported)
}

func (w *nodeVersionWorld) start(answer string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	w.ctx, w.answer, w.home = ctx, oldHomeAnswers[answer], exampleKey("a8.7 home of rc.1")
	w.cleanup(cancel)
	rvURL, err := startAcceptanceRelayRV(w.cleanup)
	if err != nil {
		return err
	}
	if err := w.startHome(rvURL); err != nil {
		return err
	}
	return w.startNode(rvURL)
}

// startHome puts the fake home on the rendezvous; it takes a connection from
// any device.
func (w *nodeVersionWorld) startHome(rvURL string) error {
	path, err := peer.Dial(w.ctx, rvURL, w.home)
	if err != nil {
		return err
	}
	w.cleanup(func() { path.Close() })
	socks, err := peer.Listen(0, 0, "test")
	if err != nil {
		return err
	}
	db, err := store.Open(":memory:")
	if err != nil {
		socks.Close()
		return err
	}
	w.cleanup(func() { socks.Close(); db.Close() })
	routes := func(string) []peer.Route { return []peer.Route{path.Route()} }
	p := peer.New(w.home, socks, db, identity.IsDevice, routes, func(c *peer.Connection) { go w.serve(c) }, nil)
	go path.Serve(w.ctx, p, func(string, []byte) error { return nil })
	return nil
}

// serve records what the node sends on one connection and answers its hello
// as the home of rc.1 does.
func (w *nodeVersionWorld) serve(c *peer.Connection) {
	w.mu.Lock()
	n := len(w.conns)
	w.conns = append(w.conns, nil)
	w.mu.Unlock()
	for {
		raw, err := c.Read(w.ctx)
		if err != nil {
			return
		}
		var hello proto.Hello
		proto.Decode(raw, &hello)
		w.note(n, hello)
		if hello.Type == "hello" {
			c.Send(json.RawMessage(fmt.Sprintf(w.answer, hello.ID)))
		}
	}
}

func (w *nodeVersionWorld) note(conn int, frame proto.Hello) {
	line := frame.Type
	if frame.Type == "hello" {
		line = fmt.Sprintf("hello v%d", frame.V)
	}
	w.mu.Lock()
	w.conns[conn] = append(w.conns[conn], line)
	w.mu.Unlock()
}

// startNode writes a data directory whose node remembers the fake home and
// uses the rendezvous, then starts the node on it.
func (w *nodeVersionWorld) startNode(rvURL string) error {
	root, err := shortTempRoot(w.f.t, w.cleanup, "sdv")
	if err != nil {
		return err
	}
	w.data = filepath.Join(root, "solodawn")
	if runtime.GOOS == "darwin" {
		w.data = filepath.Join(root, "Library", "Application Support", "solodawn")
	}
	if err := w.seed(rvURL); err != nil {
		return err
	}
	cmd := exec.Command(w.f.bin, "node")
	cmd.Env = append(os.Environ(), "HOME="+root, "XDG_DATA_HOME="+root, "XDG_CONFIG_HOME="+root, "LocalAppData="+root)
	cmd.Stdout, cmd.Stderr = &w.output, &w.output
	if _, err := startNodeCmd(w.f.t, w.cleanup, cmd, "node", w.data, &w.output); err != nil {
		return err
	}
	return waitAcceptanceSocket(filepath.Join(w.data, "sock"), 10*time.Second)
}

func (w *nodeVersionWorld) seed(rvURL string) error {
	if err := os.MkdirAll(w.data, 0o700); err != nil {
		return err
	}
	db, err := store.Open(filepath.Join(w.data, "node.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	homes := accJSON([]proto.Home{{Group: exampleID("a8.7 group"), Home: w.home.ID(), Name: "rc.1"}})
	return db.Write(func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('homes', ?), ('rendezvous', ?)", homes, accJSON([]string{rvURL}))
		return err
	})
}

// waitTwoConnections waits for the node's second connection that carries
// something: it reconnects after its back-off of 1 s and then 2 s
// (internal/peer/reconnect.go).
func (w *nodeVersionWorld) waitTwoConnections() error {
	return waitUntil(15*time.Second, "two connections from the node", func() error {
		got := w.connections()
		if used := slices.DeleteFunc(slices.Clone(got), func(frames []string) bool { return len(frames) == 0 }); len(used) < 2 {
			return fmt.Errorf("the node sent %q", got)
		}
		return nil
	})
}

func (w *nodeVersionWorld) connections() [][]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([][]string, len(w.conns))
	for i, frames := range w.conns {
		out[i] = slices.Clone(frames)
	}
	return out
}

func (w *nodeVersionWorld) checkHellos() error {
	var errs []error
	for i, frames := range w.connections() {
		if len(frames) != 0 && !slices.Equal(frames, []string{"hello v3"}) {
			errs = append(errs, fmt.Errorf("connection %d carried %q", i+1, frames))
		}
	}
	return errors.Join(errs...)
}

func (w *nodeVersionWorld) checkReported() error {
	want := "the home " + w.home.ID() + " speaks another protocol version: update this node and that computer together"
	log, err := os.ReadFile(filepath.Join(w.data, "logs", "node.log"))
	if err != nil || !strings.Contains(string(log), want) {
		return fmt.Errorf("the node's log does not say %q (%v):\n%s", want, err, log)
	}
	return nil
}
