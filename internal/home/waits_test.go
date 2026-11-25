package home_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// timedOut is the failure of a wait that ran out: its time, what it was for,
// and the last 20 of what it saw.
func timedOut(d time.Duration, what string, saw []string) string {
	return fmt.Sprintf("timed out after %s waiting for %s; saw %d: %s", d, what, len(saw), strings.Join(saw[max(0, len(saw)-20):], ", "))
}

// poll calls check every millisecond until it returns "". What it returns
// instead is what it saw; each reading that differs from the one before is
// listed when the wait runs out.
func poll(t testing.TB, d time.Duration, what string, check func() string) {
	t.Helper()
	var saw []string
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	timeout := time.After(d)
	for {
		got := check()
		if got == "" {
			return
		}
		if len(saw) == 0 || saw[len(saw)-1] != got {
			saw = append(saw, got)
		}
		select {
		case <-tick.C:
		case <-timeout:
			t.Fatal(timedOut(d, what, saw))
		}
	}
}
