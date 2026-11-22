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

// Protocol step v 2 covers steps 2 and 3, while v 1 keeps decoding for the
// fallback to older nodes (contracts/step-3/knowledge.md section 8).
func TestHelloAndWelcomeSupportV2AndV1Fallback(t *testing.T) {
	var hello Hello
	if err := Decode(example(t, "hello"), &hello); err != nil || hello.V != 2 {
		t.Fatalf("hello = %+v, %v", hello, err)
	}
	var welcome Welcome
	if err := Decode(example(t, "welcome"), &welcome); err != nil || welcome.V != 2 {
		t.Fatalf("welcome = %+v, %v", welcome, err)
	}
	v1Hello := strings.Replace(string(example(t, "hello")), `"v": 2`, `"v": 1`, 1)
	if err := Decode([]byte(v1Hello), &hello); err != nil || hello.V != 1 {
		t.Errorf("v 1 hello = %+v, %v", hello, err)
	}
	v1Welcome := strings.Replace(string(example(t, "welcome")), `"v": 2`, `"v": 1`, 1)
	if err := Decode([]byte(v1Welcome), &welcome); err != nil || welcome.V != 1 {
		t.Errorf("v 1 welcome = %+v, %v", welcome, err)
	}
}
