package proto

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A success has "ok": true and nothing else there (contracts/conventions.md section 3).
func TestSuccessIsOnlyTrue(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "welcome.json"))
	if err != nil {
		t.Fatal(err)
	}
	var welcome Welcome
	if err := Decode(data, &welcome); err != nil || !welcome.OK {
		t.Fatalf("welcome: %+v, %v", welcome, err)
	}
	for _, ok := range []string{"false", `"true"`, "1"} {
		changed := strings.Replace(string(data), `"ok": true`, `"ok": `+ok, 1)
		if err := Decode([]byte(changed), &welcome); err == nil {
			t.Errorf("ok %s: read as %+v", ok, welcome.Success)
		}
	}
	var answer CmdResult
	if err := Decode([]byte(`{"re":4,"ok":false}`), &answer); err != errNotTrue {
		t.Errorf("a cmd answer with ok false: %v, want %v", err, errNotTrue)
	}
}
