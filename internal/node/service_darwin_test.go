package node

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestServiceLoginAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	plist := filepath.Join(home, "Library", "LaunchAgents", "org.solodawn.node.plist")
	serviceOK(t, InstallService("/opt/old", "/usr/bin"))
	serviceOK(t, InstallService("/opt/a <node> & b", "/opt/a&b/bin:/usr/bin"))
	serviceFile(t, plist, `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>Label</key><string>org.solodawn.node</string>
  <key>ProgramArguments</key><array><string>/opt/a &lt;node&gt; &amp; b</string><string>node</string></array>
  <key>AssociatedBundleIdentifiers</key><string>org.solodawn.node</string>
  <key>EnvironmentVariables</key><dict><key>PATH</key><string>/opt/a&amp;b/bin:/usr/bin</string></dict>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`)
	// launchd reads the agent through the same property list parser as plutil.
	out, err := exec.Command("plutil", "-convert", "json", "-o", "-", plist).Output()
	serviceOK(t, err)
	var got map[string]any
	serviceOK(t, json.Unmarshal(out, &got))
	want := map[string]any{
		"AssociatedBundleIdentifiers": "org.solodawn.node",
		"EnvironmentVariables":        map[string]any{"PATH": "/opt/a&b/bin:/usr/bin"},
		"Label":                       "org.solodawn.node",
		"ProgramArguments":            []any{"/opt/a <node> & b", "node"},
		"RunAtLoad":                   true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plutil: got %v; want %v", got, want)
	}
	serviceOK(t, RemoveService())
	serviceAbsent(t, plist)
}

func TestServiceAgentPathErrors(t *testing.T) {
	t.Setenv("HOME", "")
	serviceError(t, InstallService("/opt/node", "/usr/bin"), "$HOME is not defined")
	serviceError(t, RemoveService(), "$HOME is not defined")

	home := t.TempDir()
	t.Setenv("HOME", home)
	file := filepath.Join(home, "Library")
	serviceOK(t, os.WriteFile(file, nil, 0600))
	serviceError(t, InstallService("/opt/node", "/usr/bin"), "mkdir "+file+": not a directory")

	home2 := t.TempDir()
	t.Setenv("HOME", home2)
	plistDir := filepath.Join(home2, "Library", "LaunchAgents", "org.solodawn.node.plist")
	serviceOK(t, os.MkdirAll(plistDir, 0700))
	serviceError(t, InstallService("/opt/node", "/usr/bin"), "open "+plistDir+": is a directory")
}

func TestStartAgentAndExecLaunchctl(t *testing.T) {
	savedBin, savedRun, savedUID := launchctlBin, runLaunchctl, getUID
	t.Cleanup(func() {
		launchctlBin = savedBin
		runLaunchctl = savedRun
		getUID = savedUID
	})
	fakeDir := t.TempDir()
	okBin := filepath.Join(fakeDir, "launchctl")
	launchctlBin = okBin
	serviceOK(t, os.WriteFile(okBin, []byte("#!/bin/sh\nexit 0\n"), 0700))
	serviceOK(t, execLaunchctl("bootstrap", "gui/501", "/tmp/test.plist"))

	serviceOK(t, os.WriteFile(okBin, []byte("#!/bin/sh\necho 'service' >&2\necho failed >&2\nexit 7\n"), 0700))
	serviceError(t, execLaunchctl("bootout", "gui/501/org.solodawn.node"), "launchctl bootout: exit status 7 (service failed)")

	getUID = func() int { return 501 }

	var calls [][]string
	runLaunchctl = func(args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		if args[0] == "bootout" {
			return errors.New("not loaded")
		}
		return nil
	}
	serviceOK(t, startAgent("/tmp/agent.plist"))
	wantCalls := [][]string{
		{"bootout", "gui/501/org.solodawn.node"},
		{"bootstrap", "gui/501", "/tmp/agent.plist"},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %v; want %v", calls, wantCalls)
	}

	failErr := errors.New("bootstrap failed")
	runLaunchctl = func(args ...string) error {
		if args[0] == "bootout" {
			return nil
		}
		return failErr
	}
	if err := startAgent("/tmp/agent.plist"); !errors.Is(err, failErr) {
		t.Fatalf("startAgent bootstrap err = %v; want %v", err, failErr)
	}
}

// bootout answers while the old node is still stopping: launchd keeps the
// service, and refuses bootstrap, until that process has ended.
func TestStartAgentWaitsUntilLaunchdHasRemovedTheOldService(t *testing.T) {
	const service = "gui/501/org.solodawn.node"
	calls, pauses := fakeStoppingAgent(t, 3)
	serviceOK(t, startAgent("/tmp/agent.plist"))
	want := []string{"bootout " + service, "print " + service, "print " + service, "print " + service, "print " + service, "bootstrap gui/501 /tmp/agent.plist"}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls = %q; want %q", *calls, want)
	}
	if want := []time.Duration{50 * time.Millisecond, 50 * time.Millisecond, 50 * time.Millisecond}; !reflect.DeepEqual(*pauses, want) {
		t.Fatalf("pauses = %v; want %v", *pauses, want)
	}
}

// A service that launchd still has 10 seconds after bootout is given up:
// 200 pauses of 50 ms, and no bootstrap.
func TestStartAgentGivesUpWhenLaunchdKeepsTheOldService(t *testing.T) {
	calls, pauses := fakeStoppingAgent(t, 1000)
	serviceError(t, startAgent("/tmp/agent.plist"), "launchd did not remove gui/501/org.solodawn.node within 10s of bootout")
	if len(*pauses) != 200 || len(*calls) != 202 || (*calls)[201] != "print gui/501/org.solodawn.node" {
		t.Fatalf("%d pauses and %d calls, the last %q; want 200 and 202, the last a print", len(*pauses), len(*calls), (*calls)[len(*calls)-1])
	}
}

// fakeStoppingAgent stands in for launchd with a loaded agent whose node
// takes stopping answers of print to stop after bootout, and for the pause
// between two of them. It returns the launchctl calls and the pauses.
func fakeStoppingAgent(t *testing.T, stopping int) (*[]string, *[]time.Duration) {
	savedRun, savedUID, savedPause := runLaunchctl, getUID, agentPause
	t.Cleanup(func() {
		runLaunchctl = savedRun
		getUID = savedUID
		agentPause = savedPause
	})
	getUID = func() int { return 501 }
	var calls []string
	var pauses []time.Duration
	agentPause = func(d time.Duration) { pauses = append(pauses, d) }
	runLaunchctl = func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		switch {
		case args[0] == "bootout":
			return nil
		case args[0] == "print" && stopping > 0:
			stopping--
			return nil
		case args[0] == "print":
			return errors.New("launchctl print: exit status 113 (Could not find service)")
		case stopping > 0:
			return errors.New("launchctl bootstrap: exit status 5 (Bootstrap failed: 5: Input/output error)")
		}
		return nil
	}
	return &calls, &pauses
}
