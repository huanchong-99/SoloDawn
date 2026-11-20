package fakecli

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
)

type server struct {
	Command string
	Args    []string
}

func claudeServer(args []string) (server, error) {
	i := slices.Index(args, "--mcp-config")
	if i < 0 || i+1 == len(args) {
		return server{}, errors.New("expected --mcp-config <file>")
	}
	b, err := os.ReadFile(args[i+1])
	if err != nil {
		return server{}, err
	}
	var cfg struct{ MCPServers map[string]server }
	if err := json.Unmarshal(b, &cfg); err != nil {
		return server{}, err
	}
	return cfg.MCPServers["solodawn"], nil
}

func codexServer(args []string) (server, error) {
	var srv server
	for i := 0; i < len(args); i++ {
		if args[i] != "-c" {
			continue
		}
		i++
		if i == len(args) {
			return srv, errors.New("expected key=value after -c")
		}
		key, value, _ := strings.Cut(args[i], "=")
		switch key {
		case "mcp_servers.solodawn.command":
			if err := json.Unmarshal([]byte(value), &srv.Command); err != nil {
				return srv, err
			}
		case "mcp_servers.solodawn.args":
			if err := json.Unmarshal([]byte(value), &srv.Args); err != nil {
				return srv, err
			}
		}
	}
	return srv, nil
}

type mcpProcess struct {
	cmd    *exec.Cmd
	input  io.WriteCloser
	output *bufio.Reader
}

func (p *mcpProcess) start(vendor string, args []string) error {
	config := claudeServer
	if vendor == "codex" {
		config = codexServer
	}
	srv, err := config(args)
	if err != nil {
		return err
	}
	if srv.Command == "" {
		return errors.New("missing solodawn MCP command")
	}
	cmd := exec.Command(srv.Command, srv.Args...)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd, p.input, p.output = cmd, stdin, bufio.NewReader(stdout)
	return nil
}

func (p *mcpProcess) close() {
	if p.cmd == nil {
		return
	}
	p.input.Close()
	// A scripted crash can strand the shim in an unanswered call.
	p.cmd.Process.Kill()
	p.cmd.Wait()
}
