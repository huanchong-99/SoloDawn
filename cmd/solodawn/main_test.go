package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCLI(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "solodawn.exe")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(bin, "version").CombinedOutput()
	if err != nil || string(out) != "dev\n" {
		t.Fatalf("version: got %q, %v; want dev and success", out, err)
	}
	for _, args := range [][]string{nil, {"unknown"}, {"version", "extra"}} {
		out, err := exec.Command(bin, args...).CombinedOutput()
		if err == nil || string(out) != "usage: solodawn version\n" {
			t.Errorf("args %q: got %q, %v; want usage and failure", args, out, err)
		}
	}
}
