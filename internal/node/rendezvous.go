package node

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
)

// The glob includes the placeholder in builds made before the web build.
//
//go:embed webdist/*
var assets embed.FS

// Rendezvous runs the standalone signalling and static HTTP server.
func Rendezvous(addr string) error {
	web, _ := fs.Sub(assets, "webdist") // a valid, constant fs path
	return http.ListenAndServe(addr, rendezvous.Handler(web))
}
