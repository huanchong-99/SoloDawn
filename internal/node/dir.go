package node

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// DataDir returns the current user's SoloDawn data directory.
func DataDir() (string, error) {
	return dataDir(runtime.GOOS)
}

func dataDir(system string) (string, error) {
	switch system {
	case "windows":
		root := os.Getenv("LocalAppData")
		if root == "" {
			return "", errors.New("LocalAppData is not set")
		}
		return filepath.Join(root, "solodawn"), nil
	case "darwin":
		return underHome("Library/Application Support")
	default:
		if root := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(root) {
			return filepath.Join(root, "solodawn"), nil
		}
		return underHome(".local/share")
	}
}

func underHome(subdir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, subdir, "solodawn"), nil
}
