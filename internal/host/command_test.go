package host

import (
	"testing"
	"time"
)

// The default stays pinned here: a test that shortens the delay cannot see a
// changed initializer.
func TestCommandWaitDelayDefault(t *testing.T) {
	if commandWaitDelay != 10*time.Second {
		t.Fatalf("wait delay %v, want 10s", commandWaitDelay)
	}
}
