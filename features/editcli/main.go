//go:build acceptance

// Command editcli stands in for the agent CLI it is named after and edits
// README.md in its working directory on every turn. It holds back only where
// what SoloDawn passed to it makes the vendor's CLI hold back (the documented
// half of the read-only level that the vendor enforces), and then logs a
// refusal. The rest of the conversation is the script of the fake CLI.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/host/fakecli"
)

var versions = map[string]string{"claude": "2.1.311 (Claude Code)", "codex": "codex-cli 0.164.0", "opencode": "1.19.6", "qwen": "0.28.0"}

func main() {
	vendor := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	args := os.Args[1:]
	var stdin io.Reader = os.Stdin
	var stdout io.Writer = os.Stdout
	switch vendor {
	case "claude":
		stdin = tap(func(line []byte) { onClaude(args, line) })
	case "codex":
		stdin = tap(new(codex).on)
	default:
		stdout = &front{vendor: vendor}
	}
	os.Exit(fakecli.Run(vendor, versions[vendor], args, stdin, stdout, os.Stderr))
}

// tap passes stdin on line by line, each line first to on.
func tap(on func([]byte)) io.Reader {
	r, w := io.Pipe()
	go func() {
		in := bufio.NewReader(os.Stdin)
		for {
			line, err := in.ReadBytes('\n')
			on(line)
			w.Write(line)
			if err != nil {
				w.CloseWithError(err)
				return
			}
		}
	}()
	return r
}

// Claude Code has only the tools of --tools, and all of them (also "default")
// without the flag. It holds back only if every tool listed is read-only.
func onClaude(args []string, line []byte) {
	var msg struct{ Type string }
	if json.Unmarshal(line, &msg) != nil || msg.Type != "user" {
		return
	}
	i := slices.Index(args, "--tools")
	if i >= 0 && readOnly(strings.Split(args[i+1], ",")) {
		edit("claude", "--tools "+args[i+1])
		return
	}
	edit("claude", "")
}

func readOnly(tools []string) bool {
	return !slices.ContainsFunc(tools, func(tool string) bool {
		return !slices.Contains([]string{"Read", "Glob", "Grep", "AskUserQuestion"}, tool)
	})
}

// Codex runs every command and patch of a thread in the sandbox of the
// thread/start request.
type codex struct{ sandbox string }

func (c *codex) on(line []byte) {
	var msg struct {
		Method string
		Params struct{ Sandbox string }
	}
	if json.Unmarshal(line, &msg) != nil {
		return
	}
	switch msg.Method {
	case "thread/start":
		c.sandbox = msg.Params.Sandbox
	case "turn/start":
		held := ""
		if c.sandbox == "read-only" {
			held = "sandbox " + c.sandbox
		}
		edit("codex", held)
	}
}

// front is the stdout of a served CLI: it puts a proxy before the server that
// the line names, so that a prompt is seen as it arrives.
type front struct{ vendor string }

var address = regexp.MustCompile(`http://[0-9.:]+`)

func (f *front) Write(line []byte) (int, error) {
	inner := address.Find(line)
	target, err := url.Parse(string(inner))
	if err != nil || inner == nil {
		return os.Stdout.Write(line)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/prompt") {
			edit(f.vendor, held[f.vendor]())
		}
		proxy.ServeHTTP(w, r)
	}))
	os.Stdout.Write(bytes.Replace(line, inner, []byte(srv.URL), 1))
	return len(line), nil
}

// held says which setting forbids an edit, or "".
var held = map[string]func() string{
	"opencode": func() string {
		var config struct{ Permission struct{ Edit, Bash string } }
		json.Unmarshal([]byte(os.Getenv("OPENCODE_CONFIG_CONTENT")), &config)
		if config.Permission.Edit == "deny" && config.Permission.Bash == "deny" {
			return "permission.edit and permission.bash deny"
		}
		return ""
	},
	"qwen": func() string {
		var settings struct{ Tools struct{ ApprovalMode string } }
		raw, _ := os.ReadFile(os.Getenv("QWEN_CODE_SYSTEM_SETTINGS_PATH"))
		json.Unmarshal(raw, &settings)
		if settings.Tools.ApprovalMode == "plan" {
			return "tools.approvalMode plan"
		}
		return ""
	},
}

func edit(vendor, held string) {
	dir, _ := os.Getwd()
	path := filepath.Join(dir, "README.md")
	outcome := "refused"
	if held == "" {
		outcome, held = "wrote", "nothing"
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err == nil {
			fmt.Fprintln(file, "edited by", vendor)
			err = file.Close()
		}
		if err != nil {
			outcome, held = "failed", err.Error()
		}
	}
	log, err := os.OpenFile(os.Getenv("SOLODAWN_FAKECLI_LOG"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer log.Close()
	fmt.Fprintf(log, "edit %s %s by %q at %q\n", vendor, outcome, held, path)
}
