package main

import (
	"os"

	"github.com/huanchong-99/SoloDawn/internal/host/fakecli"
)

func main() {
	os.Exit(fakecli.Run("codex", "codex-cli 0.164.0", os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
