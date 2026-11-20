package host

import (
	"testing"
	"time"
)

func TestCommandLimits(t *testing.T) {
	for _, c := range []struct {
		name string
		args []string
		want time.Duration
	}{
		{"git", []string{"clone", "--branch", "main", "--", "src", "dst"}, 60 * time.Minute},
		{"git", []string{"push", "origin", "refs/heads/x:refs/heads/x"}, 60 * time.Minute},
		{"git", []string{"fetch", "origin"}, 10 * time.Minute},
		{"git", []string{"pull"}, 10 * time.Minute},
		{"git", nil, 10 * time.Minute},
		{"gh", []string{"push"}, 10 * time.Minute},
		{"gh", []string{"pr", "merge", "1"}, 10 * time.Minute},
	} {
		if got := limitOf(c.name, c.args); got != c.want {
			t.Errorf("%s %q: limit %v, want %v", c.name, c.args, got, c.want)
		}
	}
}
