package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const keychainService = "provider-switcher"

// keychainTimeout bounds `security` calls: with a locked keychain the CLI
// can sit on a GUI unlock prompt indefinitely.
const keychainTimeout = 60 * time.Second

// All Keychain access goes through the system `security` CLI so we take no
// Keychain API dependency. Tests fake it by putting shims on PATH.

func securityCmd(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "security", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return out, fmt.Errorf("security %v timed out: %w", args, ctx.Err())
		}
		return out, fmt.Errorf("security %v: %w: %s", args, err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func keychainStore(name, key string) error {
	_, err := securityCmd("add-generic-password",
		"-s", keychainService, "-a", name, "-w", key, "-U")
	if err != nil {
		return fmt.Errorf("storing key for %q in Keychain: %w", name, err)
	}
	return nil
}

func keychainGet(name string) (string, error) {
	out, err := securityCmd("find-generic-password",
		"-s", keychainService, "-a", name, "-w")
	if err != nil {
		return "", fmt.Errorf("no key for %q in Keychain: run `psw set-key %s`: %w",
			name, name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func keychainDelete(name string) error {
	if _, err := securityCmd("delete-generic-password",
		"-s", keychainService, "-a", name); err != nil {
		return fmt.Errorf("deleting key for %q from Keychain: %w", name, err)
	}
	return nil
}

// stdinReader is a seam so tests can feed input without a terminal.
// It must be a single long-lived reader: bufio buffers ahead, so creating
// a fresh reader per prompt would lose the buffered lines.
var stdinReader = bufio.NewReader(os.Stdin)

// stdinIsTerminal reports whether the real stdin (not the test seam) is a
// terminal. Only a terminal has echo to hide.
func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// promptSecret prints prompt to out, reads a line with terminal echo off,
// and restores echo. The key never appears on screen. When stdin is piped
// there is no terminal echo to hide, so stty is skipped entirely.
func promptSecret(prompt string, out io.Writer) (string, error) {
	fmt.Fprint(out, prompt)
	if stdinIsTerminal() {
		if err := runStty("-echo"); err != nil {
			return "", err
		}
		defer func() { _ = runStty("echo") }()
	}

	line, err := stdinReader.ReadString('\n')
	fmt.Fprintln(out)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("reading input: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// runStty runs stty against the real terminal (stdin), not the injected
// input seam, so tests can shim it via PATH.
func runStty(args ...string) error {
	cmd := exec.Command("stty", args...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("stty %v: %w", args, err)
	}
	return nil
}
