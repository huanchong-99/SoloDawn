package node

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServiceLoginUnit(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	unit := filepath.Join(config, "systemd", "user", "solodawn.service")
	link := filepath.Join(filepath.Dir(unit), "default.target.wants", "solodawn.service")
	serviceOK(t, InstallService("/opt/an old installation"))
	serviceOK(t, InstallService(`/opt/%n/$HOME/a "node"`))
	want := "[Unit]\nDescription=SoloDawn node\n\n[Service]\n" +
		"ExecStart=:/bin/sh -c 'exec \"$0\" node' \"/opt/%%n/$HOME/a \\\"node\\\"\"\n\n[Install]\nWantedBy=default.target\n"
	serviceFile(t, unit, want)
	target, err := filepath.EvalSymlinks(link)
	serviceOK(t, err)
	if target != unit {
		t.Fatalf("login link resolves to %q; want %q", target, unit)
	}
	serviceOK(t, RemoveService())
	serviceAbsent(t, link)
	serviceAbsent(t, unit)
}

func TestServiceMissingConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	const missing = "neither $XDG_CONFIG_HOME nor $HOME are defined"
	serviceError(t, InstallService("/opt/node"), missing)
	serviceError(t, RemoveService(), missing)
}

func TestServiceUnitWriteErrors(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	file := filepath.Join(config, "systemd")
	serviceOK(t, os.WriteFile(file, nil, 0600))
	serviceError(t, InstallService("/opt/node"), "mkdir "+file+": not a directory")
	serviceOK(t, os.Remove(file))
	unit := filepath.Join(file, "user", "solodawn.service")
	serviceOK(t, os.MkdirAll(unit, 0700))
	serviceError(t, InstallService("/opt/node"), "open "+unit+": is a directory")
}

func TestServiceManagerFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("PATH", dir)
	serviceOK(t, os.WriteFile(filepath.Join(dir, "systemctl"), []byte("#!/bin/sh\necho 'unit manager' >&2\necho failed >&2\nexit 7\n"), 0700))
	serviceError(t, InstallService("/opt/node"), "systemctl enable: exit status 7 (unit manager failed)")
	serviceError(t, RemoveService(), "systemctl disable: exit status 7 (unit manager failed)")
}
