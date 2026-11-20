package main

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestServiceCommand(t *testing.T) {
	self, _ := os.Executable() // as service reads it
	t.Setenv("PATH", "/opt/tools/bin:/usr/bin")
	const usage = "usage: solodawn <command> [arguments]\n"
	for _, tc := range []struct {
		args   []string
		fail   error
		code   int
		stderr string
		calls  []string
	}{
		{[]string{"install"}, nil, 0, "", []string{"install " + self + " /opt/tools/bin:/usr/bin"}},
		{[]string{"remove"}, nil, 0, "", []string{"remove"}},
		{[]string{"install"}, errors.New("cannot write\nthe entry"), 1, "cannot write the entry\n", []string{"install " + self + " /opt/tools/bin:/usr/bin"}},
		{[]string{"remove"}, errors.New("no entry"), 1, "no entry\n", []string{"remove"}},
		{nil, nil, 2, usage, nil},
		{[]string{"start"}, nil, 2, usage, nil},
		{[]string{"install", "extra"}, nil, 2, usage, nil},
	} {
		var calls []string
		cmd := service(
			func(executable, path string) error {
				calls = append(calls, "install "+executable+" "+path)
				return tc.fail
			},
			func() error { calls = append(calls, "remove"); return tc.fail },
		)
		var stdout, stderr bytes.Buffer
		code := cmd(tc.args, &stdout, &stderr)
		if code != tc.code || stdout.Len() != 0 || stderr.String() != tc.stderr || !reflect.DeepEqual(calls, tc.calls) {
			t.Errorf("service %q = %d, stdout %q, stderr %q, calls %q; want %d, %q, %q", tc.args, code, stdout.String(), stderr.String(), calls, tc.code, tc.stderr, tc.calls)
		}
	}
}

// The table entry is registered; only arguments that never reach the
// system's service manager are dispatched here.
func TestServiceIsRegistered(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"service", "status"}, &stdout, &stderr); code != 2 || stdout.Len() != 0 || stderr.String() != "usage: solodawn <command> [arguments]\n" {
		t.Fatalf("service status = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}
