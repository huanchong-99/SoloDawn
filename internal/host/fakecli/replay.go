// Package fakecli runs the test-only, file-scripted Claude and Codex processes.
package fakecli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
)

type step struct {
	Dir string
	Msg json.RawMessage
}

type script struct {
	Session string // the thread id issued by the scripted Codex server
	Steps   []step
	Exit    int
}

func Main(vendor, version string) {
	args := os.Args[1:]
	if slices.Equal(args, []string{"--version"}) {
		fmt.Println(version)
		return
	}
	code, err := run(vendor, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake "+vendor+":", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func sessionID(vendor string, args []string, id string) (string, error) {
	switch {
	case vendor == "claude":
		for i, arg := range args {
			if arg == "--session-id" || arg == "--resume" {
				if i+1 < len(args) {
					return args[i+1], nil
				}
				break
			}
		}
		return "", errors.New("expected --session-id or --resume with an id")
	case len(args) > 1 && args[0] == "resume":
		return args[1], nil
	case len(args) > 0 && args[0] == "app-server" && id != "":
		return id, nil
	default:
		return "", errors.New("expected app-server with script session, or resume <id>")
	}
}

func run(vendor string, args []string) (code int, err error) {
	key := "SOLODAWN_FAKE_" + strings.ToUpper(vendor) + "_SCRIPT"
	path := os.Getenv(key)
	if path == "" {
		return 0, fmt.Errorf("set %s to a script file", key)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var s script
	if err := json.Unmarshal(data, &s); err != nil {
		return 0, err
	}
	if s.Steps == nil {
		return 0, errors.New("script requires a steps array")
	}
	id, err := sessionID(vendor, args, s.Session)
	if err != nil {
		return 0, err
	}
	log, err := os.OpenFile(os.Getenv("SOLODAWN_FAKECLI_LOG"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return 0, err
	}
	defer log.Close()
	if _, err := fmt.Fprintln(log, "start", vendor, id); err != nil {
		return 0, err
	}
	defer func() {
		_, logErr := fmt.Fprintln(log, "exit", vendor, id)
		err = errors.Join(err, logErr)
	}()
	session, _ := json.Marshal(id)
	p := replay{stdin: bufio.NewReader(os.Stdin), session: session}
	defer p.mcp.close()
	for i, step := range s.Steps {
		if err := p.step(step, vendor, args); err != nil {
			return 0, fmt.Errorf("step %d (%s): %w", i+1, step.Dir, err)
		}
	}
	return s.Exit, nil
}

type replay struct {
	stdin              *bufio.Reader
	session, requestID json.RawMessage
	mcp                mcpProcess
}

func (p *replay) step(s step, vendor string, args []string) error {
	if s.Dir != "eof" && len(s.Msg) == 0 {
		return errors.New("missing msg")
	}
	msg := bytes.ReplaceAll(s.Msg, []byte(`"$session"`), p.session)
	switch s.Dir {
	case "stdin":
		got, err := expect(p.stdin, msg)
		p.requestID = got["id"]
		return err
	case "stdout":
		if p.requestID == nil && bytes.Contains(msg, []byte(`"$id"`)) {
			return errors.New("no request id on the last stdin message")
		}
		return json.NewEncoder(os.Stdout).Encode(json.RawMessage(bytes.ReplaceAll(msg, []byte(`"$id"`), p.requestID)))
	case "mcp<-cli":
		if p.mcp.cmd == nil {
			if err := p.mcp.start(vendor, args); err != nil {
				return err
			}
		}
		return json.NewEncoder(p.mcp.input).Encode(json.RawMessage(msg))
	case "mcp->cli":
		if p.mcp.cmd == nil {
			return errors.New("no open MCP transport")
		}
		_, err := expect(p.mcp.output, msg)
		return err
	case "eof":
		_, err := p.stdin.ReadByte()
		if err == io.EOF {
			return nil
		}
		return errors.New("expected closed stdin")
	case "stderr":
		var line string
		if err := json.Unmarshal(msg, &line); err != nil {
			return err
		}
		_, err := fmt.Fprintln(os.Stderr, line)
		return err
	default:
		return fmt.Errorf("unknown direction %q", s.Dir)
	}
}

// Read one NDJSON message. Object expectations can name just the fields a test
// cares about; a missing field never matches null.
func expect(in *bufio.Reader, msg json.RawMessage) (map[string]json.RawMessage, error) {
	line, err := in.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil {
		return nil, err
	}
	if !contains(messageValue(line), messageValue(msg)) {
		return nil, fmt.Errorf("expected %s, got %s", msg, line)
	}
	return fields, nil
}

// MCP carries tool results as JSON inside text. Go and the capture's Python
// server need not put spaces or object keys in the same places.
func messageValue(line []byte) any {
	v := valueOf(line)
	obj, _ := v.(map[string]any)
	result, _ := obj["result"].(map[string]any)
	content, _ := result["content"].([]any)
	for _, item := range content {
		block, _ := item.(map[string]any)
		text, ok := block["text"].(string)
		if ok && block["type"] == "text" && json.Valid([]byte(text)) {
			type jsonText struct{ value any }
			block["text"] = jsonText{valueOf([]byte(text))}
		}
	}
	return v
}

// Both the script and the incoming line have already been parsed as JSON.
func valueOf(b []byte) any {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		panic(err)
	}
	return v
}

func contains(got, want any) bool {
	fields, ok := want.(map[string]any)
	if !ok {
		return reflect.DeepEqual(got, want)
	}
	obj, ok := got.(map[string]any)
	if !ok {
		return false
	}
	for k, v := range fields {
		actual, present := obj[k]
		if !present || !contains(actual, v) {
			return false
		}
	}
	return true
}
