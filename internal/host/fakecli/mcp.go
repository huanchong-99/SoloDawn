package fakecli

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
)

type server struct {
	Command string
	Args    []string
}

type mcp struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out *bufio.Reader
}

// The first MCP step starts the server the node configured as "solodawn".
func (p *process) mcpStep(dir string, msg []byte) error {
	if p.mcp == nil {
		if err := p.startMCP(); err != nil {
			return err
		}
	}
	if dir == "mcp<-cli" {
		return send(p.mcp.in, msg)
	}
	_, err := expect(p.mcp.out, msg)
	return err
}

func (p *process) startMCP() error {
	srv, err := p.server()
	if err != nil {
		return err
	}
	cmd := exec.Command(srv.Command, srv.Args...)
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	p.mcp = &mcp{cmd, in, bufio.NewReader(out)}
	return nil
}

func (p *process) stopMCP() {
	if p.mcp == nil {
		return
	}
	p.mcp.in.Close()
	// A script that ends inside a call leaves the server waiting.
	p.mcp.cmd.Process.Kill()
	p.mcp.cmd.Wait()
}

// Claude Code names a configuration file; Codex takes -c settings, whose TOML
// values are also JSON here.
func (p *process) server() (srv server, err error) {
	if path := value(p.args, "--mcp-config"); path != "" {
		var config struct{ MCPServers map[string]server }
		data, err := os.ReadFile(path)
		if err == nil {
			err = json.Unmarshal(data, &config)
		}
		return config.MCPServers["solodawn"], err
	}
	for _, arg := range p.args {
		if value, ok := strings.CutPrefix(arg, "mcp_servers.solodawn.command="); ok {
			err = errors.Join(err, json.Unmarshal([]byte(value), &srv.Command))
		}
		if value, ok := strings.CutPrefix(arg, "mcp_servers.solodawn.args="); ok {
			err = errors.Join(err, json.Unmarshal([]byte(value), &srv.Args))
		}
	}
	return srv, err
}
