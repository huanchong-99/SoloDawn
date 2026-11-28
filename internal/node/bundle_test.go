package node

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestBundleOf(t *testing.T) {
	for _, tc := range []struct {
		exe  string
		want string
	}{
		{filepath.FromSlash("/Applications/SoloDawn.app/Contents/MacOS/solodawn"), filepath.FromSlash("/Applications/SoloDawn.app")},
		{filepath.FromSlash("/Applications/SoloDawn.app/Contents/MacOS/SoloDawnPermissions"), filepath.FromSlash("/Applications/SoloDawn.app")},
		{filepath.FromSlash("SoloDawn.app/Contents/MacOS/solodawn"), "SoloDawn.app"},
		{filepath.FromSlash("/Applications/SoloDawn.app/Contents/MacOS/../MacOS/solodawn/"), filepath.FromSlash("/Applications/SoloDawn.app")},
		{"", ""},
		{filepath.FromSlash("/usr/local/bin/solodawn"), ""},
		{filepath.FromSlash("/Applications/SoloDawn.app/Contents/MacOS"), ""},
		{filepath.FromSlash("/Applications/SoloDawn.app/Contents/MacOS/sub/solodawn"), ""},
		{filepath.FromSlash("/Applications/SoloDawn.app/Other/MacOS/solodawn"), ""},
		{filepath.FromSlash("/Applications/SoloDawn.app/Contents/Other/solodawn"), ""},
		{filepath.FromSlash("/Applications/SoloDawn/Contents/MacOS/solodawn"), ""},
	} {
		if got := bundleOf(tc.exe); got != tc.want {
			t.Errorf("bundleOf(%q) = %q; want %q", tc.exe, got, tc.want)
		}
	}
}

func TestIsBundled(t *testing.T) {
	savedExe := executablePath
	t.Cleanup(func() { executablePath = savedExe })

	executablePath = func() (string, error) { return "", errors.New("no exe") }
	if IsBundled() {
		t.Fatal("IsBundled = true on error")
	}
	executablePath = func() (string, error) { return filepath.FromSlash("/usr/local/bin/solodawn"), nil }
	if IsBundled() {
		t.Fatal("IsBundled = true outside bundle")
	}
	executablePath = func() (string, error) {
		return filepath.FromSlash("/Applications/SoloDawn.app/Contents/MacOS/solodawn"), nil
	}
	if !IsBundled() {
		t.Fatal("IsBundled = false inside bundle")
	}
}

func TestLaunch(t *testing.T) {
	savedSpawn, savedWait, savedOpen := spawnNode, pairStartWait, openURL
	t.Cleanup(func() {
		spawnNode = savedSpawn
		pairStartWait = savedWait
		openURL = savedOpen
	})

	for _, name := range []string{"HOME", "XDG_DATA_HOME", "LocalAppData", "USERPROFILE"} {
		t.Setenv(name, "")
	}
	if err := Launch(); err == nil {
		t.Fatal("Launch without DataDir = nil, want error")
	}

	dir := nodeTestDir(t)
	pairStartWait = 2 * time.Second
	wantErr := errors.New("spawn failed")
	spawnNode = func(bool) error { return wantErr }
	if err := Launch(); !errors.Is(err, wantErr) {
		t.Fatalf("Launch spawn error = %v; want %v", err, wantErr)
	}

	var bg *Local
	t.Cleanup(func() {
		if bg != nil {
			bg.Close()
		}
	})
	spawns := 0
	spawnNode = func(rewrite bool) error {
		if rewrite {
			return errors.New("Launch overwrote agent plist")
		}
		spawns++
		var err error
		bg, err = ListenLocal(dir)
		return err
	}
	var opened []string
	openURL = func(u string) error {
		opened = append(opened, u)
		return nil
	}
	if err := Launch(); err != nil {
		t.Fatalf("Launch cold = %v", err)
	}
	if spawns != 1 || len(opened) != 1 || opened[0] != "http://localhost:47800" {
		t.Fatalf("cold launch: spawns=%d opened=%v", spawns, opened)
	}

	if err := Launch(); err != nil {
		t.Fatalf("Launch warm = %v", err)
	}
	if spawns != 1 || len(opened) != 2 || opened[1] != "http://localhost:47800" {
		t.Fatalf("warm launch: spawns=%d opened=%v", spawns, opened)
	}

	openErr := errors.New("browser failed")
	openURL = func(string) error { return openErr }
	if err := Launch(); !errors.Is(err, openErr) {
		t.Fatalf("Launch openURL error = %v; want %v", err, openErr)
	}
}
