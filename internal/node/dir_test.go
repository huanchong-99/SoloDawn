package node

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestDataDirectory(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"HOME", "USERPROFILE", "LocalAppData"} {
		t.Setenv(name, root)
	}
	for _, tc := range []struct{ system, xdg, relative string }{
		{"linux", "", ".local/share"},
		{"linux", "relative", ".local/share"},
		{"linux", filepath.Join(root, "data"), "data"},
		{"darwin", "", "Library/Application Support"},
		{"windows", "", ""},
	} {
		t.Setenv("XDG_DATA_HOME", tc.xdg)
		got, err := dataDir(tc.system)
		want := filepath.Join(root, tc.relative, "solodawn")
		if err != nil || got != want {
			t.Errorf("dataDir(%q) = %q, %v; want %q", tc.system, got, err, want)
		}

	}
}

func TestNativeDataDirectory(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"HOME", "LocalAppData", "XDG_DATA_HOME", "USERPROFILE"} {
		t.Setenv(name, root)
	}
	want, err := dataDir(runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DataDir(); got != want || err != nil {
		t.Errorf("DataDir() = %q, %v; want %q", got, err, want)
	}
}

func TestMissingDataRoot(t *testing.T) {
	for _, name := range []string{"HOME", "USERPROFILE", "LocalAppData", "XDG_DATA_HOME"} {
		t.Setenv(name, "")
	}
	for _, system := range []string{"linux", "darwin", "windows"} {
		if got, err := dataDir(system); got != "" || err == nil {
			t.Errorf("dataDir(%q) = %q, %v; want empty path and error", system, got, err)
		}
	}
}
