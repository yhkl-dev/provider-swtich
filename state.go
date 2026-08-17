package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func activePath() string { return filepath.Join(configDir(), "active") }

// setActive points the active symlink at providers/<name>.
// The symlink is the entire state: no parsing, readlink is the answer.
func setActive(name string) error {
	if !providerExists(name) {
		return fmt.Errorf("unknown provider %q: run `psw list` to see providers", name)
	}
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	// Remove first so re-pointing an existing symlink cannot nest inside it.
	_ = os.Remove(activePath())
	return os.Symlink(filepath.Join("providers", name), activePath())
}

// getActive returns the active provider name.
func getActive() (string, error) {
	target, err := os.Readlink(activePath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no active provider: run `psw use <name>`")
		}
		return "", fmt.Errorf("reading active provider: %w", err)
	}
	return filepath.Base(target), nil
}
