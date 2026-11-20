package main

import (
	"bytes"
	"fmt"
	"testing"
)

func TestRun(t *testing.T) {
	const usage = "usage: solodawn version | rendezvous [--listen address]\n"
	const flags = "Usage of rendezvous:\n  -listen string\n    \tHTTP signalling and static file address (default \"127.0.0.1:8080\")\n"
	for _, tc := range []struct {
		args           []string
		code           int
		stdout, stderr string
	}{
		{[]string{"version"}, 0, "dev\n", ""},
		{nil, 2, "", usage},
		{[]string{"rendezvous", "extra"}, 2, "", flags},
		{[]string{"rendezvous", "--unknown"}, 2, "", "flag provided but not defined: -unknown\n" + flags},
		{[]string{"rendezvous", "--listen", ":9000"}, 1, "", "stopped :9000\n"},
		{[]string{"rendezvous"}, 1, "", "stopped 127.0.0.1:8080\n"},
		{[]string{"unknown"}, 2, "", usage},
		{[]string{"version", "extra"}, 2, "", usage},
	} {
		var stdout, stderr bytes.Buffer
		code := run(tc.args, &stdout, &stderr, func(addr string) error { return fmt.Errorf("stopped %s", addr) })
		if code != tc.code || stdout.String() != tc.stdout || stderr.String() != tc.stderr {
			t.Errorf("run(%q) = %d, stdout %q, stderr %q; want %d, %q, %q", tc.args, code, stdout.String(), stderr.String(), tc.code, tc.stdout, tc.stderr)
		}
	}
}
