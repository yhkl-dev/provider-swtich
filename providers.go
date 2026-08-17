package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// kv is one KEY=VALUE line from a provider file.
type kv struct {
	key   string
	value string
}

// configDir returns the state directory. PSW_CONFIG_DIR overrides it
// (used by tests and end-to-end verification).
func configDir() string {
	if d := os.Getenv("PSW_CONFIG_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// $HOME is unset; fall back to a relative path so we still work.
		return filepath.Join(".config", "provider-switcher")
	}
	return filepath.Join(home, ".config", "provider-switcher")
}

func providersDir() string { return filepath.Join(configDir(), "providers") }

func providerPath(name string) string { return filepath.Join(providersDir(), name) }

// validateName rejects names that cannot safely be file names.
func validateName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid provider name %q", name)
	}
	if strings.ContainsAny(name, "/ \t\n") {
		return fmt.Errorf("invalid provider name %q: must not contain '/' or whitespace", name)
	}
	return nil
}

// parseProvider reads KEY=VALUE lines, skipping blank lines and # comments.
// Key and value are split on the first '='; value may contain '='.
func parseProvider(r io.Reader) ([]kv, error) {
	kvs := []kv{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("malformed line %q: expected KEY=VALUE", line)
		}
		kvs = append(kvs, kv{strings.TrimSpace(key), strings.TrimSpace(value)})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading provider file: %w", err)
	}
	return kvs, nil
}

// saveProvider writes the provider file, creating the providers dir as needed.
func saveProvider(name string, kvs []kv) error {
	if err := validateName(name); err != nil {
		return err
	}
	if err := os.MkdirAll(providersDir(), 0o700); err != nil {
		return err
	}
	var b strings.Builder
	for _, e := range kvs {
		fmt.Fprintf(&b, "%s=%s\n", e.key, e.value)
	}
	return os.WriteFile(providerPath(name), []byte(b.String()), 0o600)
}

func providerExists(name string) bool {
	_, err := os.Stat(providerPath(name))
	return err == nil
}

// listProviders returns provider names, sorted. No providers dir is not an error.
func listProviders() ([]string, error) {
	entries, err := os.ReadDir(providersDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
