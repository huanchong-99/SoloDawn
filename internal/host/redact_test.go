package host

import (
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestRedactSecrets(t *testing.T) {
	ghp := "ghp_" + strings.Repeat("a", 40)
	sk := "sk-proj-" + strings.Repeat("b", 32)
	for _, tc := range []struct{ text, want string }{
		{"", ""},
		{"nothing secret here\nA=1\nB=2\nC=3", "nothing secret here\nA=1\nB=2\nC=3"},
		{"token " + ghp + " end", "token [redacted] end"},
		{"(" + sk + ") and " + sk, "([redacted]) and [redacted]"},
		{"AWS_AKIA" + strings.Repeat("C", 20) + "\n", "AWS_[redacted]\n"},
		{"github_pat_" + strings.Repeat("d", 30) + ".x", "[redacted].x"},
		{"ghs_1_" + strings.Repeat("e", 30) + ".f.g end", "[redacted] end"},
		{"before\n-----BEGIN RSA PRIVATE KEY-----\nMIIab\n-----END RSA PRIVATE KEY-----\nafter", "before\n[redacted]\nafter"},
		{"a -----BEGIN PRIVATE KEY-----\nMIIab\nrest", "a [redacted]"},
		{"-----BEGIN PRIVATE KEY-----\n1\n-----END PRIVATE KEY-----\nmid\n-----BEGIN EC PRIVATE KEY-----\n2\n-----END EC PRIVATE KEY-----", "[redacted]\nmid\n[redacted]"},
		{"intro\nA=1\napi_key=2\nC=3\noutro", "intro\n[redacted]\noutro"},
		{"A=1\nB=2\nC=3\nD=4\n\nX=1\nMY_TOKEN=2\nZ=3", "A=1\nB=2\nC=3\nD=4\n\n[redacted]"},
	} {
		if got := redactSecrets(tc.text); got != tc.want {
			t.Errorf("redactSecrets(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

func TestCheckOutputIsRedacted(t *testing.T) {
	command := "echo key ghp_" + strings.Repeat("a", 36)
	line := "key [redacted]\n"
	if runtime.GOOS == "windows" {
		line = "key [redacted]\r\n"
	}
	want := proto.CheckItem{Name: command, OK: true, Detail: &line}
	if got := runProjectCheck(t.Context(), t.TempDir(), command); !reflect.DeepEqual(got, want) {
		t.Fatalf("check item %q %v %q, want detail %q", got.Name, got.OK, *got.Detail, line)
	}
}
