package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
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

// CheckRunner runs a task's project checks and both hard checks. The secrets
// check applies CheckOutboundSecret, the patterns of local.md section 8, to
// the added lines.
type CheckRunner struct{}

var ErrDirtyCheckTree = errors.New("conflict: the worktree has uncommitted changes")

// Run executes the group's checks in order, followed by both hard checks
// against the same commit. A failed check is an item, not a runner error.
func (r CheckRunner) Run(ctx context.Context, worktree, baseRef string, group proto.GroupRow) (proto.ChecksBody, error) {
	head, err := cleanCheckHead(ctx, worktree)
	if err != nil {
		return proto.ChecksBody{}, err
	}
	result := proto.ChecksBody{Stage: "local", Commit: head, OK: true}
	result.Items = runGroupChecks(ctx, worktree, group)
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

func runGroupChecks(ctx context.Context, dir string, group proto.GroupRow) []proto.CheckItem {
	checkCtx, cancel := context.WithTimeout(ctx, time.Duration(group.Limits.TurnMinutes)*time.Minute)
	defer cancel()
	var items []proto.CheckItem
	for _, command := range group.Checks {
		items = append(items, runProjectCheck(checkCtx, dir, command))
	}
	return items
}

func checkGit(ctx context.Context, dir string, args ...string) (string, error) {
	out, stderr, err := runIn(ctx, dir, "git", args...)
	if _, ok := errors.AsType[*exec.ExitError](err); ok {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr))
	}
	return out, err
}

func cleanCheckHead(ctx context.Context, dir string) (string, error) {
	status, err := checkGit(ctx, dir, "status", "--porcelain", "--untracked-files=all")
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
	// The tail goes to the group, so a secret in it is redacted (local.md 8).
	if len(tail) != 0 {
		item.Detail = new(redactSecrets(string(tail)))
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
		checkedItem("secrets", checkAddedLines(patch)),
		checkedItem("tests", checkTestFiles(names)),
	}, nil
}

// '>' marks added lines only. Headers, context and deletions separate runs
// so the multiline secret matcher never combines unrelated additions.
var addedCheckLines = regexp.MustCompile(`(?m)^(?:>[^\n]*\n?)+`)

func checkAddedLines(patch string) error {
	for _, run := range addedCheckLines.FindAllString(patch, -1) {
		if err := CheckOutboundSecret(strings.ReplaceAll(run[1:], "\n>", "\n")); err != nil {
			return err
		}
	}
	return nil
}

// The source files of the "tests" hard check (local.md section 7).
var checkCodeExtensions = strings.Fields(".cjs .cts .dart .ex .go .js .jsx .mjs .mts .php .py .ts .tsx")

func isCheckTestFile(name string) bool {
	dir, file := path.Split(name)
	return strings.Contains("/"+dir, "/tests/") || strings.Contains("/"+dir, "/__tests__/") ||
		strings.HasPrefix(file, "test_") || strings.Contains(file, "_test.") ||
		strings.Contains(file, ".test.") || strings.Contains(file, ".spec.")
}

func checkTestFiles(names string) error {
	sourceChanged := false
	for name := range strings.SplitSeq(names, "\x00") {
		if name == "" {
			break
		}
		if isCheckTestFile(name) {
			return nil
		}
		sourceChanged = sourceChanged || slices.Contains(checkCodeExtensions, strings.ToLower(path.Ext(name)))
	}
	if sourceChanged {
		return errors.New("source changed without a test file")
	}
	return nil
}
