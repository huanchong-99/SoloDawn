package main

import (
	"fmt"
	"os"

	"github.com/huanchong-99/SoloDawn/internal/node"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "version" {
		fmt.Fprintln(os.Stderr, "usage: solodawn version")
		os.Exit(2)
	}
	fmt.Println(node.Version)
}
