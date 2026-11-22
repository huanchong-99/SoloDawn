// Package fakecli runs the test-only Claude Code and Codex stand-ins. Each
// start replays one script file of messages the real CLIs sent.
package fakecli

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
)

type script struct {
	Session string // the thread id a scripted Codex app-server issues
	Steps   []struct {
		Dir string
		Msg json.RawMessage
	}
	Exit int
}

type process struct {
	vendor         string
	args           []string
	stdin          *bufio.Reader
	stdout, stderr io.Writer
	session, id    json.RawMessage
	mcp            *mcp
}

// Run is one start of a fake CLI. It returns the exit status.
func Run(vendor, version string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if slices.Equal(args, []string{"--version"}) {
		fmt.Fprintln(stdout, version)
		return 0
	}
	p := process{vendor: vendor, args: args, stdin: bufio.NewReader(stdin), stdout: stdout, stderr: stderr}
	code, err := p.run(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "fake "+vendor+":", err)
		return 1
	}
	return code
}

func (p *process) run(rawStdin io.Reader) (int, error) {
	if p.vendor == "opencode" {
		return p.replayOpenCode(rawStdin)
	}
	return p.replay()
}

func (p *process) replay() (int, error) {
	s, err := load("SOLODAWN_FAKE_" + strings.ToUpper(p.vendor) + "_SCRIPT")
	if err != nil {
		return 0, err
	}
	id := cmp.Or(value(p.args, "--session-id", "--resume", "resume"), s.Session)
	if id == "" {
		return 0, errors.New("no session id in the arguments or the script")
	}
	log, err := os.OpenFile(os.Getenv("SOLODAWN_FAKECLI_LOG"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return 0, fmt.Errorf("SOLODAWN_FAKECLI_LOG: %w", err)
	}
	defer log.Close()
	fmt.Fprintln(log, "start", p.vendor, id)
	defer fmt.Fprintln(log, "exit", p.vendor, id)
	defer p.stopMCP()
	p.session, _ = json.Marshal(id)
	for i, step := range s.Steps {
		if err := p.step(step.Dir, step.Msg); err != nil {
			return 0, fmt.Errorf("step %d (%s): %w", i+1, step.Dir, err)
		}
	}
	return s.Exit, nil
}

func load(key string) (s script, err error) {
	data, err := os.ReadFile(os.Getenv(key))
	if err == nil {
		err = json.Unmarshal(data, &s)
	}
	if err != nil {
		err = fmt.Errorf("%s: %w", key, err)
	}
	return s, err
}

// value is the argument after the first of names.
func value(args []string, names ...string) string {
	i := slices.IndexFunc(args, func(arg string) bool { return slices.Contains(names, arg) })
	if i < 0 || i+1 == len(args) {
		return ""
	}
	return args[i+1]
}

func (p *process) step(dir string, msg []byte) error {
	msg = bytes.ReplaceAll(msg, []byte(`"$session"`), p.session)
	switch dir {
	case "stdin":
		return p.request(msg)
	case "stdout":
		return send(p.stdout, bytes.ReplaceAll(msg, []byte(`"$id"`), p.id))
	case "stderr":
		return p.print(msg)
	case "eof":
		return p.eof()
	case "mcp<-cli", "mcp->cli":
		return p.mcpStep(dir, msg)
	}
	return fmt.Errorf("unknown direction %q", dir)
}

// A reply quotes the id of the last line from the node as "$id".
func (p *process) request(want []byte) error {
	line, err := expect(p.stdin, want)
	if err != nil {
		return err
	}
	var request struct{ ID json.RawMessage }
	err = json.Unmarshal(line, &request)
	p.id = request.ID
	return err
}

func (p *process) print(msg []byte) error {
	var line string
	if err := json.Unmarshal(msg, &line); err != nil {
		return err
	}
	_, err := fmt.Fprintln(p.stderr, line)
	return err
}

func (p *process) eof() error {
	if _, err := p.stdin.ReadByte(); err != io.EOF {
		return errors.New("expected closed stdin")
	}
	return nil
}

func send(to io.Writer, msg []byte) error {
	var line bytes.Buffer
	if err := json.Compact(&line, msg); err != nil {
		return err
	}
	line.WriteByte('\n')
	_, err := to.Write(line.Bytes())
	return err
}

// expect reads one line and checks that it has every field of want.
func expect(from *bufio.Reader, want []byte) ([]byte, error) {
	line, err := from.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	line = line[:len(line)-1]
	var got, fields any
	if err := errors.Join(json.Unmarshal(line, &got), json.Unmarshal(want, &fields)); err != nil {
		return nil, err
	}
	if !contains(got, fields) {
		return nil, fmt.Errorf("expected %s, got %s", want, line)
	}
	return line, nil
}

// An object matches when it has the wanted fields; anything else must be equal.
func contains(got, want any) bool {
	fields, ok := want.(map[string]any)
	if !ok {
		return reflect.DeepEqual(got, want)
	}
	object, ok := got.(map[string]any)
	for name, value := range fields {
		if !contains(object[name], value) {
			return false
		}
	}
	return ok
}
