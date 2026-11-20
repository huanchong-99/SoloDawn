package node

import (
	"embed"
	"io/fs"

	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
)

// The glob includes the placeholder in builds made before the web build.
//
//go:embed webdist/*
var assets embed.FS

type RendezvousOptions = rendezvous.Options

// Rendezvous runs the standalone signalling, static file and ICE service.
func Rendezvous(opts RendezvousOptions) error {
	web, _ := fs.Sub(assets, "webdist") // a valid, constant fs path
	return opts.Serve(web)
}
