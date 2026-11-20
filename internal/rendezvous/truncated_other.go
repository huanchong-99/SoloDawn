//go:build !windows

package rendezvous

// truncated is false here: these systems cut a long datagram without an
// error, and the read fills the whole buffer.
func truncated(error) bool { return false }
