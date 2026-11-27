package node

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// serviceSteps records the calls of installServiceFile and removeServiceFile
// to their path and register functions.
type serviceSteps struct {
	file    string
	pathErr error
	hookErr error
	calls   []string
}

func (s *serviceSteps) path() (string, error) {
	s.calls = append(s.calls, "path")
	return s.file, s.pathErr
}

// hook registers or unregisters. It records whether the file is there at
// that moment.
func (s *serviceSteps) hook() error {
	if _, err := os.Stat(s.file); err == nil {
		s.calls = append(s.calls, "hook with file")
	} else {
		s.calls = append(s.calls, "hook without file")
	}
	return s.hookErr
}

func serviceCalls(t *testing.T, s *serviceSteps, want ...string) {
	t.Helper()
	if len(s.calls) != len(want) {
		t.Fatalf("calls %q; want %q", s.calls, want)
	}
	for i := range want {
		if s.calls[i] != want[i] {
			t.Fatalf("calls %q; want %q", s.calls, want)
		}
	}
}

// The file is written in a new private folder, then registered, and the
// answer of the registration is returned.
func TestInstallServiceFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a", "b", "unit")
	s := &serviceSteps{file: file}
	serviceOK(t, installServiceFile(s.path, "text\n", s.hook))
	serviceCalls(t, s, "path", "hook with file")
	serviceFile(t, file, "text\n")
	if runtime.GOOS != "windows" {
		serviceMode(t, filepath.Dir(file), fs.ModeDir|0700)
		serviceMode(t, file, 0600)
	}
	failed := errors.New("manager failed")
	s = &serviceSteps{file: file, hookErr: failed}
	if err := installServiceFile(s.path, "other\n", s.hook); err != failed {
		t.Fatalf("got error %v; want %v", err, failed)
	}
	serviceCalls(t, s, "path", "hook with file")
	serviceFile(t, file, "other\n")
}

// Each failure is returned as it is, and nothing after it happens.
func TestInstallServiceFileErrors(t *testing.T) {
	missing := errors.New("no config folder")
	s := &serviceSteps{pathErr: missing}
	if err := installServiceFile(s.path, "text\n", s.hook); err != missing {
		t.Fatalf("got error %v; want %v", err, missing)
	}
	serviceCalls(t, s, "path")

	dir := t.TempDir()
	parent := filepath.Join(dir, "systemd")
	serviceOK(t, os.WriteFile(parent, nil, 0600))
	s = &serviceSteps{file: filepath.Join(parent, "user", "unit")}
	var pathErr *fs.PathError
	if err := installServiceFile(s.path, "text\n", s.hook); !errors.As(err, &pathErr) || pathErr.Op != "mkdir" || pathErr.Path != parent {
		t.Fatalf("got error %v; want mkdir %s", err, parent)
	}
	serviceCalls(t, s, "path")

	unit := filepath.Join(dir, "unit")
	serviceOK(t, os.Mkdir(unit, 0700))
	s = &serviceSteps{file: unit}
	if err := installServiceFile(s.path, "text\n", s.hook); !errors.As(err, &pathErr) || pathErr.Op != "open" || pathErr.Path != unit {
		t.Fatalf("got error %v; want open %s", err, unit)
	}
	serviceCalls(t, s, "path")
}

// The file is unregistered while it is still there, then removed.
func TestRemoveServiceFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "unit")
	serviceOK(t, os.WriteFile(file, nil, 0600))
	s := &serviceSteps{file: file}
	serviceOK(t, removeServiceFile(s.path, s.hook))
	serviceCalls(t, s, "path", "hook with file")
	serviceAbsent(t, file)

	s = &serviceSteps{file: file}
	if err := removeServiceFile(s.path, s.hook); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("got error %v; want a missing file", err)
	}
	serviceCalls(t, s, "path", "hook without file")
}

// A failure to find or to unregister the file is returned as it is, and the
// file stays.
func TestRemoveServiceFileErrors(t *testing.T) {
	missing := errors.New("no config folder")
	s := &serviceSteps{pathErr: missing}
	if err := removeServiceFile(s.path, s.hook); err != missing {
		t.Fatalf("got error %v; want %v", err, missing)
	}
	serviceCalls(t, s, "path")

	file := filepath.Join(t.TempDir(), "unit")
	serviceOK(t, os.WriteFile(file, nil, 0600))
	failed := errors.New("manager failed")
	s = &serviceSteps{file: file, hookErr: failed}
	if err := removeServiceFile(s.path, s.hook); err != failed {
		t.Fatalf("got error %v; want %v", err, failed)
	}
	serviceCalls(t, s, "path", "hook with file")
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("file after a failed unregistration: %v", err)
	}
}

func serviceMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	serviceOK(t, err)
	if info.Mode() != want {
		t.Fatalf("%s: mode %v; want %v", path, info.Mode(), want)
	}
}
