package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// CheckRunner uses the host's shared secret checker and source/test path
// conventions. Neither hard check is configurable by the group.
type CheckRunner struct {
	CheckSecret  func(text string) error
	ClassifyPath func(path string) (source, test bool)
}

var ErrDirtyCheckTree = errors.New("conflict: the worktree has uncommitted changes")

// Run executes the group's checks in order, followed by both hard checks
// against the same commit. A failed check is an item, not a runner error.
func (r CheckRunner) Run(ctx context.Context, worktree, baseRef string, group proto.GroupRow) (proto.ChecksBody, error) {
	head, err := cleanCheckHead(ctx, worktree)
	if err != nil {
		return proto.ChecksBody{}, err
	}
	result := proto.ChecksBody{Stage: "local", Commit: head, OK: true}
	result.Items = runGroupChecks(ctx, worktree, head, group)
	hard, err := r.hardChecks(ctx, worktree, baseRef+"..."+head)
	if err != nil {
		return proto.ChecksBody{}, err
	}
	result.Items = append(result.Items, hard...)
	for _, item := range result.Items {
		result.OK = result.OK && item.OK
	}
	return result, nil
}

func runGroupChecks(ctx context.Context, dir, head string, group proto.GroupRow) []proto.CheckItem {
	checkCtx, cancel := context.WithTimeout(ctx, time.Duration(group.Limits.TurnMinutes)*time.Minute)
	defer cancel()
	var items []proto.CheckItem
	for _, command := range group.Checks {
		item := runProjectCheck(checkCtx, dir, command)
		current, err := cleanCheckHead(ctx, dir)
		if err != nil {
			return append(items, checkedItem(command, err))
		}
		if current != head {
			return append(items, checkedItem(command, errors.New("conflict: HEAD changed during checks")))
		}
		items = append(items, item)
	}
	return items
}

func checkGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = checkEnvironment(os.Environ(), runtime.GOOS == "windows")
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(exit.Stderr)))
	}
	return string(out), err
}

func cleanCheckHead(ctx context.Context, dir string) (string, error) {
	status, err := checkGit(ctx, dir, "status", "--porcelain", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return "", err
	}
	if status != "" {
		return "", ErrDirtyCheckTree
	}
	head, err := checkGit(ctx, dir, "rev-parse", "--verify", "HEAD")
	return strings.TrimSpace(head), err
}

func checkEnvironment(env []string, windows bool) []string {
	allowed := []string{"PATH", "HOME", "USER", "LOGNAME", "SHELL", "LANG", "TZ", "TMPDIR", "TEMP", "TMP"}
	if windows {
		allowed = append(allowed, "SYSTEMROOT", "COMSPEC", "PATHEXT", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "APPDATA", "LOCALAPPDATA")
	}
	result := []string{}
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if windows {
			name = strings.ToUpper(name)
		}
		if slices.Contains(allowed, name) || strings.HasPrefix(name, "LC_") {
			result = append(result, entry)
		}
	}
	return result
}

const checkTailLimit = 4096

type checkTail []byte

func (t *checkTail) Write(p []byte) (int, error) {
	n := len(p)
	p = p[max(0, n-checkTailLimit):]
	*t = append((*t)[max(0, len(*t)+len(p)-checkTailLimit):], p...)
	return n, nil
}

func runProjectCheck(ctx context.Context, dir, command string) proto.CheckItem {
	attrs := new(syscall.SysProcAttr)
	cmd := checkShell(ctx, command, runtime.GOOS == "windows", reflect.ValueOf(attrs).Elem())
	cmd.SysProcAttr = attrs
	cmd.WaitDelay = time.Second
	cmd.Dir = dir
	cmd.Env = checkEnvironment(os.Environ(), runtime.GOOS == "windows")
	var tail checkTail
	cmd.Stdout, cmd.Stderr = &tail, &tail
	err := cmd.Run()
	item := checkedItem(command, err)
	if len(tail) != 0 {
		item.Detail = new(string(tail))
	}
	return item
}

// Go exposes CmdLine only on Windows and Setpgid only on Unix. Set the
// native attribute before starting the shell. CmdLine preserves cmd.exe's
// quoting; a separate Unix process group lets cancellation reach a pipeline.
func checkShell(ctx context.Context, command string, windows bool, fields reflect.Value) *exec.Cmd {
	if windows {
		cmd := exec.CommandContext(ctx, os.Getenv("ComSpec"))
		fields.FieldByName("CmdLine").SetString(`cmd.exe /d /s /c "` + command + `"`)
		cmd.Cancel = func() error {
			return exec.Command("taskkill.exe", "/f", "/t", "/pid", strconv.Itoa(cmd.Process.Pid)).Run()
		}
		return cmd
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	fields.FieldByName("Setpgid").SetBool(true)
	cmd.Cancel = func() error {
		group, _ := os.FindProcess(-cmd.Process.Pid) // FindProcess always succeeds on Unix.
		return group.Kill()
	}
	return cmd
}

func checkedItem(name string, err error) proto.CheckItem {
	item := proto.CheckItem{Name: name, OK: err == nil}
	if err != nil {
		item.Detail = new(err.Error())
	}
	return item
}

func (r CheckRunner) hardChecks(ctx context.Context, dir, revisions string) ([]proto.CheckItem, error) {
	patch, err := checkGit(ctx, dir, "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--find-renames", "--unified=0", "--output-indicator-new=>", "--end-of-options", revisions, "--")
	if err != nil {
		return nil, err
	}
	names, err := checkGit(ctx, dir, "diff", "--name-only", "-z", "--no-renames", "--end-of-options", revisions, "--")
	if err != nil {
		return nil, err
	}
	return []proto.CheckItem{
		checkedItem("secrets", r.checkAddedLines(patch)),
		checkedItem("tests", r.checkTestFiles(names)),
	}, nil
}

// '>' marks added lines only. Headers, context and deletions separate runs
// so the multiline secret matcher never combines unrelated additions.
var addedCheckLines = regexp.MustCompile(`(?m)^(?:>[^\n]*\n?)+`)

func (r CheckRunner) checkAddedLines(patch string) error {
	for _, run := range addedCheckLines.FindAllString(patch, -1) {
		if err := r.CheckSecret(strings.ReplaceAll(run[1:], "\n>", "\n")); err != nil {
			return err
		}
	}
	return nil
}

func (r CheckRunner) checkTestFiles(names string) error {
	sourceChanged := false
	for name := range strings.SplitSeq(names, "\x00") {
		if name == "" {
			break
		}
		source, test := r.ClassifyPath(name)
		if test {
			return nil
		}
		sourceChanged = sourceChanged || source
	}
	if sourceChanged {
		return errors.New("source changed without a test file")
	}
	return nil
}
