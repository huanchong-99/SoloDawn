package node

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// The events of planning/contracts/diagnostics.md section 2 that the node
// writes. Every line goes through nodeRuntime.diag, a logger with the node's
// id. A line holds ids, counts and states.

func diagDevice(id string) string { return id[:min(len(id), 8)] }

// diagLevel reads logs/level: the word debug, or anything else for info.
func diagLevel(dir string) slog.Level {
	raw, _ := os.ReadFile(filepath.Join(dir, "logs", "level"))
	if strings.TrimSpace(string(raw)) == "debug" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

// diagOutput points log at w and returns the function that puts it back.
func diagOutput(w io.Writer, level slog.Level) func() {
	out, flags, was := log.Writer(), log.Flags(), slog.SetLogLoggerLevel(level)
	log.SetOutput(w)
	log.SetFlags(log.Ldate | log.Lmicroseconds | log.LUTC)
	return func() {
		log.SetOutput(out)
		log.SetFlags(flags)
		slog.SetLogLoggerLevel(was)
	}
}

func (n *nodeRuntime) diagNode(event string) {
	n.diag.Info(event, "version", Version, "os", runtime.GOOS, "level", strings.ToLower(n.level.String()))
}

// diagBG writes bg.start and returns the function that writes bg.stop. The
// goroutine calls it, because the caller may hold a mutex, and defers the stop
// after its wait group's Done, so that Close sees it.
func (n *nodeRuntime) diagBG(name string) func() {
	n.diag.Info("bg.start", "name", name)
	return func() { n.diag.Info("bg.stop", "name", name) }
}
