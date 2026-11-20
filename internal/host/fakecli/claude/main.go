package main

import (
	"os"

	"github.com/huanchong-99/SoloDawn/internal/host/fakecli"
)

func main() {
	os.Exit(fakecli.Run("claude", "2.1.311 (Claude Code)", os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
