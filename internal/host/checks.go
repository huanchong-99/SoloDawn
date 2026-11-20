package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"reflect"
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

var (
	ErrDirtyCheckTree = errors.New("conflict: the worktree has uncommitted changes")
	// ErrSkipWorktree is wrapped with up to 10 of the paths.
	ErrSkipWorktree = errors.New("conflict: Git hides these paths from the status with skip-worktree, as in a sparse checkout")
	errHardTimeout  = errors.New("timed out")
)

// The hard checks have their own limit instead of the turn's (R-103), and
// the secrets check reads at most this much of the added lines.
var (
	hardCheckLimit = 2 * time.Minute
	addedLineLimit = 67108864 // 64 MiB
)

// Run executes the group's checks in order, followed by both hard checks
// against the same commit. A failed check is an item, not a runner error.
// An explicit cancel of ctx (a pause, an interrupt, a shutdown) stops the
// run, which then reports nothing and returns that error; the turn's
// deadline stops only the group's checks.
func (r CheckRunner) Run(ctx context.Context, worktree, baseRef string, group proto.GroupRow) (proto.ChecksBody, error) {
	result, err := r.run(ctx, worktree, baseRef, group)
	if errors.Is(ctx.Err(), context.Canceled) {
		return proto.ChecksBody{}, ctx.Err()
	}
	return result, err
}

func (r CheckRunner) run(ctx context.Context, worktree, baseRef string, group proto.GroupRow) (proto.ChecksBody, error) {
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

// The group's checks share the time left in the turn: ctx carries the
// turn's deadline, and turn_minutes bounds a context without one.
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
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(exit.Stderr)))
	}
	return string(out), err
}

// cleanCheckHead refuses a tree whose status is not empty (local.md 7,
// run_checks). A path with the skip-worktree bit is refused by name, as its
// content never shows; the assume-unchanged bit, a speed hint that
// core.ignoreStat sets on every path, is cleared first, so that a change
// shows.
func cleanCheckHead(ctx context.Context, dir string) (string, error) {
	tracked, err := checkGit(ctx, dir, "ls-files", "-v", "-z")
	if err != nil {
		return "", err
	}
	skip, assume := hiddenCheckPaths(tracked)
	if len(skip) != 0 {
		return "", skipWorktreeError(skip)
	}
	if err := clearAssumeUnchanged(ctx, dir, assume); err != nil {
		return "", err
	}
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

// hiddenCheckPaths reads `git ls-files -v -z`: tag S or s for
// skip-worktree, any other lower-case tag for assume-unchanged.
func hiddenCheckPaths(tracked string) (skip, assume []string) {
	for entry := range strings.SplitSeq(strings.TrimSuffix(tracked, "\x00"), "\x00") {
		tag, name, _ := strings.Cut(entry, " ")
		if strings.EqualFold(tag, "S") {
			skip = append(skip, name)
		} else if tag != strings.ToUpper(tag) {
			assume = append(assume, name)
		}
	}
	return skip, assume
}

func skipWorktreeError(paths []string) error {
	shown := strings.Join(paths[:min(len(paths), 10)], ", ")
	if len(paths) > 10 {
		shown += fmt.Sprintf(", and %d more", len(paths)-10)
	}
	return fmt.Errorf("%w: %s", ErrSkipWorktree, shown)
}

// clearAssumeUnchanged clears the bit of paths in the worktree's index. The
// paths go through stdin, NUL-terminated, however many there are.
func clearAssumeUnchanged(ctx context.Context, dir string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	cmd := exec.CommandContext(ctx, "git", "update-index", "--no-assume-unchanged", "-z", "--stdin")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git update-index: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
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

// A check's output is kept as its last 64 KiB, redacted, and then cut to
// the 4 KiB tail the group sees: redacting before the cut keeps a key that
// spans the cut from leaving its end in the tail.
const (
	checkOutputWindow = 64 << 10
	checkTailLimit    = 4096
)

type checkTail []byte

func (t *checkTail) Write(p []byte) (int, error) {
	n := len(p)
	p = p[max(0, n-checkOutputWindow):]
	*t = append((*t)[max(0, len(*t)+len(p)-checkOutputWindow):], p...)
	return n, nil
}

func runProjectCheck(ctx context.Context, dir, command string) proto.CheckItem {
	attrs := new(syscall.SysProcAttr)
	cmd := checkShell(ctx, command, runtime.GOOS == "windows", reflect.ValueOf(attrs).Elem())
	cmd.SysProcAttr = attrs
	cmd.WaitDelay = time.Second
	cmd.Dir = dir
	cmd.Env = checkEnvironment(os.Environ(), runtime.GOOS == "windows")
	var output checkTail
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Start()
	if err == nil {
		kill := killCheckTree(cmd)
		err = cmd.Wait()
		// A background child of the check dies with it.
		kill()
	}
	return proto.CheckItem{Name: command, OK: err == nil, Detail: checkDetail(err, string(output))}
}

// checkDetail is the error, then the tail of the output, which goes to the
// group and so is redacted first (local.md 7 and 8).
func checkDetail(err error, output string) *string {
	var parts []string
	if err != nil {
		parts = append(parts, err.Error())
	}
	if output != "" {
		output = RedactSecrets(output)
		parts = append(parts, output[max(0, len(output)-checkTailLimit):])
	}
	if parts == nil {
		return nil
	}
	return new(strings.Join(parts, "\n"))
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
	ctx, stop := hardCheckContext(ctx)
	defer stop()
	// --text: a file Git takes as binary still shows its added lines.
	patch, err := checkGit(ctx, dir, "diff", "--text", "--no-ext-diff", "--no-textconv", "--no-color", "--find-renames", "--unified=0", "--output-indicator-new=>", "--end-of-options", revisions, "--")
	secrets, err := hardItem(ctx, "secrets", err, func() error { return checkAddedLines(ctx, patch) })
	if err != nil {
		return nil, err
	}
	// --diff-filter=d: a deleted file counts as neither a test nor a source
	// file; a rename is a deletion and an addition.
	names, err := checkGit(ctx, dir, "diff", "--name-only", "-z", "--no-renames", "--diff-filter=d", "--end-of-options", revisions, "--")
	tests, err := hardItem(ctx, "tests", err, func() error { return checkTestFiles(names) })
	if err != nil {
		return nil, err
	}
	return []proto.CheckItem{secrets, tests}, nil
}

// hardCheckContext bounds the hard checks by hardCheckLimit instead of the
// turn's deadline, which they outlive (R-103). An explicit cancel of parent
// still stops them.
func hardCheckContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), hardCheckLimit)
	unwatch := context.AfterFunc(parent, func() {
		if errors.Is(parent.Err(), context.Canceled) {
			cancel()
		}
	})
	return ctx, func() { unwatch(); cancel() }
}

// hardItem is a hard check's verdict on what Git read. A read that the
// limit of the hard checks stopped fails the item with "timed out"; any
// other failed read is the runner's error.
func hardItem(ctx context.Context, name string, readErr error, verdict func() error) (proto.CheckItem, error) {
	if readErr == nil {
		return checkedItem(name, verdict()), nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return checkedItem(name, errHardTimeout), nil
	}
	return proto.CheckItem{}, readErr
}

// checkAddedLines looks for a secret in each run of added lines, in place:
// every line of a run keeps its '>'. It reads at most addedLineLimit bytes.
func checkAddedLines(ctx context.Context, patch string) error {
	runs := addedRuns(patch)
	if size := addedSize(runs); size > addedLineLimit {
		return fmt.Errorf("too large to check: %d bytes of added lines, over the limit of %d", size, addedLineLimit)
	}
	for _, run := range runs {
		if ctx.Err() != nil {
			return errHardTimeout
		}
		if containsSecret(patch[run[0]:run[1]], ">") {
			return errSecret
		}
	}
	return nil
}

// addedRuns are the runs of consecutive added lines of patch, the lines
// that start with '>'. Headers, context and deletions separate runs, so the
// multiline matcher never combines unrelated additions.
func addedRuns(patch string) [][2]int {
	var runs [][2]int
	off := 0
	for line := range strings.SplitAfterSeq(patch, "\n") {
		if strings.HasPrefix(line, ">") {
			runs = extendRun(runs, off, off+len(line))
		}
		off += len(line)
	}
	return runs
}

func extendRun(runs [][2]int, start, end int) [][2]int {
	if n := len(runs); n > 0 && runs[n-1][1] == start {
		runs[n-1][1] = end
		return runs
	}
	return append(runs, [2]int{start, end})
}

func addedSize(runs [][2]int) int {
	size := 0
	for _, run := range runs {
		size += run[1] - run[0]
	}
	return size
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
