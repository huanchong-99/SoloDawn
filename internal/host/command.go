package host

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"time"
)

func taskkill(pid int) error {
	return exec.Command("taskkill.exe", "/f", "/t", "/pid", strconv.Itoa(pid)).Run()
}

// commandUnit is the unit of the command limits: a minute. Tests shorten it.
var commandUnit = time.Minute

// commandWaitDelay is how long the output of a stopped command may stay
// open, held by a child that left its session: 10 s. Tests shorten it.
var commandWaitDelay = time.Duration(10e9)

// noPrompt turns off every way git, gh, ssh and Git Credential Manager ask a
// person. The owner's ssh command and ssh configuration are left alone.
var noPrompt = []string{"GIT_TERMINAL_PROMPT=0", "GH_PROMPT_DISABLED=1", "SSH_ASKPASS_REQUIRE=never", "GCM_INTERACTIVE=never"}

// foregroundGC is the setting that has Git do the automatic maintenance that
// follows a commit, a merge or a fetch before the command ends, added to the
// settings the node's environment gives Git. By default Git leaves it to a
// process of its own, which works on in the clone outside the clone's lock and
// outside the command's limit.
func foregroundGC() []string {
	count, _ := strconv.Atoi(os.Getenv("GIT_CONFIG_COUNT"))
	n := strconv.Itoa(count)
	return []string{"GIT_CONFIG_COUNT=" + strconv.Itoa(count+1), "GIT_CONFIG_KEY_" + n + "=gc.autoDetach", "GIT_CONFIG_VALUE_" + n + "=false"}
}

// runIn runs the git or gh command name in dir and returns what it wrote to
// standard output and to standard error. Every git and gh command of the host
// runs through it: within ctx and the command's own limit, with the node's
// environment plus noPrompt and foregroundGC, and with no controlling terminal, so it never
// waits for a person. Project check commands have their own runner.
func runIn(ctx context.Context, dir, name string, args ...string) (stdout, stderr string, err error) {
	return runInput(ctx, dir, nil, name, args...)
}

// runInput is runIn with stdin as the command's standard input.
func runInput(ctx context.Context, dir string, stdin io.Reader, name string, args ...string) (stdout, stderr string, err error) {
	ctx, cancel := context.WithTimeout(ctx, limitOf(name, args))
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = slices.Concat(os.Environ(), noPrompt, foregroundGC())
	if dir != "" {
		// With SysProcAttr set, os.StartProcess no longer checks the folder,
		// and Windows reports a missing one only as "The directory name is
		// invalid". This is that check, with the same "chdir" error on every
		// system (os.Stat names its own call, which differs on Windows).
		if _, err := os.Stat(dir); err != nil {
			err.(*fs.PathError).Op = "chdir"
			return "", "", err
		}
		// exec sets PWD itself only when it builds the environment.
		cmd.Env = append(cmd.Env, "PWD="+dir)
	}
	detachTerminal(cmd)
	// A child that outlives a stopped command cannot hold its output open.
	cmd.WaitDelay = commandWaitDelay
	var out, errOut bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, &out, &errOut
	err = cmd.Run()
	return out.String(), errOut.String(), err
}

// limitOf is the limit of one command (local.md section 7, "Commands never
// wait for a person"): 60 minutes for git clone and git push, which may move
// a whole repository, and 10 minutes for any other git or gh command.
func limitOf(name string, args []string) time.Duration {
	if name == "git" && len(args) > 0 && (args[0] == "clone" || args[0] == "push") {
		return 60 * commandUnit
	}
	return 10 * commandUnit
}
