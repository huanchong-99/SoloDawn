package host

import (
	"strings"
	"testing"
	"time"
)

// keyBegin and keyEnd split key lines in test literals, so that this file
// holds no private-key header or footer itself.
const keyBegin, keyEnd = "-----BEGIN ", "-----END "

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
		{"MIIab\ncd\n" + keyEnd + "PRIVATE KEY-----\nafter", "[redacted]\nafter"},
		{"x\n" + keyEnd + "RSA PRIVATE KEY-----\ny\n" + keyEnd + "RSA PRIVATE KEY-----\nz", "[redacted]\nz"},
		{"a\n" + keyBegin + "PGP PRIVATE KEY BLOCK-----\n\nlQOY\n=abcd\n" + keyEnd + "PGP PRIVATE KEY BLOCK-----\nb", "a\n[redacted]\nb"},
		{"lQOY\n" + keyEnd + "PGP PRIVATE KEY BLOCK-----\nb", "[redacted]\nb"},
		{keyBegin + "PGP PUBLIC KEY BLOCK-----\nmQ\n" + keyEnd + "PGP PUBLIC KEY BLOCK-----", keyBegin + "PGP PUBLIC KEY BLOCK-----\nmQ\n" + keyEnd + "PGP PUBLIC KEY BLOCK-----"},
		{"intro\nA=1\napi_key=2\nC=3\noutro", "intro\n[redacted]\noutro"},
		{"A=1\nB=2\nC=3\nD=4\n\nX=1\nMY_TOKEN=2\nZ=3", "A=1\nB=2\nC=3\nD=4\n\n[redacted]"},
	} {
		if got := RedactSecrets(tc.text); got != tc.want {
			t.Errorf("RedactSecrets(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

// The check scans by hand what redaction finds with regular expressions;
// both must agree.
func TestCheckAndRedactionAgree(t *testing.T) {
	for _, text := range []string{
		"", "A=1\nB=2\nC=3", "A=1\nB=2\nmy_key=3", "KEY=1\nB=2", "A=1\n\nTOKEN=2\nC=3",
		"A=1\r\nTop_Secret=2\r\nC=3\r\n", "2A=1\nKEY=2\nC=3", "a-b=1\nKEY=2\nC=3", "=1\nKEY=2\nC=3",
		"x\nA=1\nB=2\nC=3\nDB_PASSWORD=4\ny", "A=KEY\nB=SECRET=v\nC=TOKEN", "Ä=1\nKEY=2\nC=3",
		"a sk-" + strings.Repeat("x", 32), "ask-" + strings.Repeat("x", 32), "sk-sk-" + strings.Repeat("y", 32),
		"xsk-sk-" + strings.Repeat("y", 32), "AKIA" + strings.Repeat("Q", 15), "_ASIA" + strings.Repeat("Q", 16),
		keyBegin + "PRIVATE KEY-----", "ghr_" + strings.Repeat("z", 29) + ".",
	} {
		redacted := RedactSecrets(text) != text
		if found := containsSecret(text); found != redacted {
			t.Errorf("%q: check %v, redaction %v", text, found, redacted)
		}
	}
}

// Dense candidates, each preceded by a letter, are checked in linear time:
// four times the text may take about four times as long, not sixteen.
func TestSecretCheckIsLinear(t *testing.T) {
	fastest := func(text string) time.Duration {
		best := time.Hour
		for range 3 {
			start := time.Now()
			if err := CheckOutboundSecret(text); err != nil {
				t.Fatalf("%.10q…: %v", text, err)
			}
			best = min(best, time.Since(start))
		}
		return best
	}
	for _, unit := range []string{"task-", "xsk-"} {
		small := fastest(strings.Repeat(unit, 256<<10/len(unit)))
		large := fastest(strings.Repeat(unit, 1<<20/len(unit)))
		if large > 10*small+100*time.Millisecond {
			t.Errorf("%q: 256 KiB in %v, 1 MiB in %v", unit, small, large)
		}
	}
}
