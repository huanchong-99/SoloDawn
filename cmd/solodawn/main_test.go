package main

import (
	"bytes"
	"testing"
)

func TestRun(t *testing.T) {
	const usage = "usage: solodawn version\n"
	for _, tc := range []struct {
		args           []string
		code           int
		stdout, stderr string
	}{
		{[]string{"version"}, 0, "dev\n", ""},
		{nil, 2, "", usage},
		{[]string{"unknown"}, 2, "", usage},
		{[]string{"version", "extra"}, 2, "", usage},
	} {
		var stdout, stderr bytes.Buffer
		code := run(tc.args, &stdout, &stderr)
		if code != tc.code || stdout.String() != tc.stdout || stderr.String() != tc.stderr {
			t.Errorf("run(%q) = %d, stdout %q, stderr %q; want %d, %q, %q", tc.args, code, stdout.String(), stderr.String(), tc.code, tc.stdout, tc.stderr)
		}
	}
}
