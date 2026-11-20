package node

import (
	"errors"
	"os"
	"testing"
)

func serviceOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func serviceError(t *testing.T, got error, want string) {
	t.Helper()
	if got == nil || got.Error() != want {
		t.Fatalf("got error %v; want %q", got, want)
	}
}

func serviceFile(t *testing.T, path, want string) {
	t.Helper()
	b, err := os.ReadFile(path)
	serviceOK(t, err)
	if string(b) != want {
		t.Fatalf("%s: got %q; want %q", path, b, want)
	}
}

func serviceAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s: want no file, got %v", path, err)
	}
}
