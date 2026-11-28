package node

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStartBackgroundDarwinAndA117(t *testing.T) {
	savedRaw, savedExe, savedSpawn, savedWait, savedOpen := rawExecutable, executablePath, spawnNode, pairStartWait, openURL
	savedLaunchctl, savedUID := runLaunchctl, getUID
	t.Cleanup(func() {
		rawExecutable = savedRaw
		executablePath = savedExe
		spawnNode = savedSpawn
		pairStartWait = savedWait
		openURL = savedOpen
		runLaunchctl = savedLaunchctl
		getUID = savedUID
	})

	// Non-bundle executable falls back to startDetached (fails on missing binary without launchd).
	var calls []string
	runLaunchctl = func(args ...string) error {
		calls = append(calls, args[0])
		return nil
	}
	if err := startBackground(filepath.Join(t.TempDir(), "solodawn"), true); err == nil || len(calls) != 0 {
		t.Fatalf("non-bundle startBackground = %v, launchctl calls=%v", err, calls)
	}

	// Bare binary inside a symlinked directory (/tmp -> /private/tmp) keeps the unresolved raw path.
	checkBareSymlinkedDirKeepsRawPath(t, &calls)

	// Bundle executable with missing HOME reports error before calling launchctl.
	bundleExe := filepath.Join(t.TempDir(), "SoloDawn.app", "Contents", "MacOS", "solodawn")
	serviceOK(t, os.MkdirAll(filepath.Dir(bundleExe), 0o700))
	serviceOK(t, os.WriteFile(bundleExe, []byte("#!/bin/sh\n"), 0o700))
	t.Setenv("HOME", "")
	if err := startBackground(bundleExe, true); err == nil || len(calls) != 0 {
		t.Fatalf("bundle startBackground without HOME = %v, launchctl calls=%v", err, calls)
	}

	// A11.7: symlinked executable resolves into .app, writes LaunchAgent plist,
	// loads via launchctl bootout+bootstrap, serves pair and Launch().
	dataDir := nodeTestDir(t)
	home := os.Getenv("HOME")
	appRoot, err := filepath.EvalSymlinks(t.TempDir())
	serviceOK(t, err)
	const shellPath = "/Users/me/.local/bin:/opt/homebrew/bin:/usr/bin:/bin"
	t.Setenv("PATH", shellPath)

	appExe := filepath.Join(appRoot, "Applications", "SoloDawn.app", "Contents", "MacOS", "solodawn")
	serviceOK(t, os.MkdirAll(filepath.Dir(appExe), 0o700))
	serviceOK(t, os.WriteFile(appExe, []byte("#!/bin/sh\n"), 0o700))
	symExe := filepath.Join(appRoot, "bin", "solodawn")
	serviceOK(t, os.MkdirAll(filepath.Dir(symExe), 0o700))
	serviceOK(t, os.Symlink(appExe, symExe))
	rawExecutable = func() (string, error) { return symExe, nil }
	executablePath = func() (string, error) {
		return resolveExecutable(func() (string, error) { return symExe, nil })
	}
	if !IsBundled() {
		t.Fatal("IsBundled = false for symlink to app bundle")
	}

	plist := filepath.Join(home, "Library", "LaunchAgents", "org.solodawn.node.plist")

	var bg *Local
	t.Cleanup(func() {
		if bg != nil {
			bg.Close()
		}
	})
	getUID = func() int { return 501 }
	var launchctlArgs [][]string
	runLaunchctl = func(args ...string) error {
		launchctlArgs = append(launchctlArgs, append([]string(nil), args...))
		if args[0] == "bootstrap" {
			var listenErr error
			bg, listenErr = ListenLocal(dataDir)
			if listenErr != nil {
				return listenErr
			}
			go servePairedOnce(bg)
		}
		return nil
	}
	spawnNode = startBackgroundNode
	pairStartWait = 2e9

	name := "mac"
	var pairOut []byte
	if err := (Command{Cmd: "pair", Secret: "s", Name: &name}).Run(&byteSink{buf: &pairOut}); err != nil {
		t.Fatalf("pair A11.7 = %v", err)
	}
	if string(pairOut) != "paired\n" {
		t.Fatalf("pair output = %q", pairOut)
	}
	if len(launchctlArgs) != 2 || launchctlArgs[0][0] != "bootout" || launchctlArgs[1][0] != "bootstrap" || launchctlArgs[1][2] != plist {
		t.Fatalf("launchctl args = %v", launchctlArgs)
	}
	wantShellPlist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>Label</key><string>org.solodawn.node</string>
  <key>ProgramArguments</key><array><string>` + appExe + `</string><string>node</string></array>
  <key>AssociatedBundleIdentifiers</key><string>org.solodawn.node</string>
  <key>EnvironmentVariables</key><dict><key>PATH</key><string>` + shellPath + `</string></dict>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`
	serviceFile(t, plist, wantShellPlist)

	// N1-1: Launch() from LaunchServices (system-only PATH) keeps the existing plist's PATH.
	bg.Close()
	bg = nil
	launchctlArgs = nil
	const systemPath = "/usr/bin:/bin:/usr/sbin:/sbin"
	t.Setenv("PATH", systemPath)
	var openedURL string
	openURL = func(u string) error {
		openedURL = u
		return nil
	}
	if err := Launch(); err != nil {
		t.Fatalf("Launch existing plist = %v", err)
	}
	if len(launchctlArgs) != 2 || launchctlArgs[0][0] != "bootout" || launchctlArgs[1][0] != "bootstrap" || openedURL != "http://localhost:47800" {
		t.Fatalf("Launch existing plist: launchctl=%v opened=%q", launchctlArgs, openedURL)
	}
	serviceFile(t, plist, wantShellPlist)

	// N1-1: Launch() with no existing plist writes one and loads it via bootout+bootstrap.
	bg.Close()
	bg = nil
	serviceOK(t, os.Remove(plist))
	launchctlArgs = nil
	openedURL = ""
	if err := Launch(); err != nil {
		t.Fatalf("Launch missing plist = %v", err)
	}
	if len(launchctlArgs) != 2 || launchctlArgs[0][0] != "bootout" || launchctlArgs[1][0] != "bootstrap" || openedURL != "http://localhost:47800" {
		t.Fatalf("Launch missing plist: launchctl=%v opened=%q", launchctlArgs, openedURL)
	}
	serviceFile(t, plist, `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>Label</key><string>org.solodawn.node</string>
  <key>ProgramArguments</key><array><string>`+appExe+`</string><string>node</string></array>
  <key>AssociatedBundleIdentifiers</key><string>org.solodawn.node</string>
  <key>EnvironmentVariables</key><dict><key>PATH</key><string>`+systemPath+`</string></dict>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`)
}

type byteSink struct{ buf *[]byte }

func (s *byteSink) Write(p []byte) (int, error) {
	*s.buf = append(*s.buf, p...)
	return len(p), nil
}

func checkBareSymlinkedDirKeepsRawPath(t *testing.T, launchctlCalls *[]string) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("/tmp", "sd-raw-")
	serviceOK(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	outFile := filepath.Join(tmpDir, "argv.txt")
	rawExe := filepath.Join(tmpDir, "solodawn")
	serviceOK(t, os.WriteFile(rawExe, []byte("#!/bin/sh\nprintf '%s|%s' \"$0\" \"$*\" > '"+outFile+"'\n"), 0o700))
	rawExecutable = func() (string, error) { return rawExe, nil }
	serviceOK(t, startBackgroundNode(true))
	if len(*launchctlCalls) != 0 {
		t.Fatalf("unexpected launchctl calls for bare binary: %v", *launchctlCalls)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(outFile); err == nil && string(data) == rawExe+"|node" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	data, err := os.ReadFile(outFile)
	t.Fatalf("raw binary detached output = %q (err=%v), want %q", string(data), err, rawExe+"|node")
}
