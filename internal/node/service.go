package node

import (
	"os"
	"path/filepath"
)

// installServiceFile writes text to the file that path names, creating its
// folder, then registers it. It stops at the first error and returns it as
// it is.
func installServiceFile(path func() (string, error), text string, register func() error) error {
	file, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(file, []byte(text), 0600); err != nil {
		return err
	}
	return register()
}

// removeServiceFile unregisters the file that path names, then removes it.
// It stops at the first error and returns it as it is.
func removeServiceFile(path func() (string, error), unregister func() error) error {
	file, err := path()
	if err != nil {
		return err
	}
	if err := unregister(); err != nil {
		return err
	}
	return os.Remove(file)
}
