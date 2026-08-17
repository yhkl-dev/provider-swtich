package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const keychainService = "provider-switcher"

// keychainTimeout bounds secret-store CLI calls: with a locked keyring the
// CLI can sit on a GUI unlock prompt indefinitely.
const keychainTimeout = 60 * time.Second

// Keys live in the best secret store available on this machine, chosen at
// runtime by keychainBackend():
//
//   - macOS: the Keychain via the `security` CLI
//   - Linux: the freedesktop Secret Service (GNOME Keyring, KWallet) via
//     `secret-tool`, when installed
//   - otherwise: a 0600 file under the config dir
//
// Everything goes through system CLIs or plain files so we take no
// platform API dependency. Tests steer the choice by shimming PATH.

// keychainBackend reports where keys are stored. Detection is by binary
// presence only; it is cheap, so callers may call it more than once.
func keychainBackend() string {
	if _, err := exec.LookPath("security"); err == nil {
		return "security"
	}
	if _, err := exec.LookPath("secret-tool"); err == nil {
		return "secret-tool"
	}
	return "file"
}

// runSecretCmd runs a secret-store CLI with the shared timeout. The error
// message shows the args with -w values redacted so a failed call never
// prints the key.
func runSecretCmd(bin string, stdin io.Reader, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = stdin
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return out, fmt.Errorf("%s %v timed out: %w", bin, redactArgs(args), ctx.Err())
		}
		return out, fmt.Errorf("%s %v: %w: %s", bin, redactArgs(args), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// redactArgs masks the value after -w (the Keychain key) in arg slices
// destined for error messages.
func redactArgs(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)
	for i, a := range out {
		if a == "-w" && i+1 < len(out) {
			out[i+1] = "***"
		}
	}
	return out
}

func securityCmd(args ...string) ([]byte, error) {
	return runSecretCmd("security", nil, args...)
}

// secretToolCmd runs secret-tool; store passes the key on stdin so it
// never appears in the process list.
func secretToolCmd(stdin io.Reader, args ...string) ([]byte, error) {
	return runSecretCmd("secret-tool", stdin, args...)
}

// keyFilePath is the file-backend location. It lives under the config dir
// so PSW_CONFIG_DIR keeps tests and containers self-contained.
func keyFilePath(name string) string {
	return filepath.Join(configDir(), "keys", name)
}

func keychainStore(name, key string) error {
	switch keychainBackend() {
	case "security":
		if _, err := securityCmd("add-generic-password",
			"-s", keychainService, "-a", name, "-w", key, "-U"); err != nil {
			return fmt.Errorf("storing key for %q in Keychain: %w", name, err)
		}
	case "secret-tool":
		if _, err := secretToolCmd(strings.NewReader(key), "store",
			"--label", "psw: "+name, "service", keychainService, "account", name); err != nil {
			return fmt.Errorf("storing key for %q in the secret store: %w", name, err)
		}
	default:
		if err := os.MkdirAll(filepath.Dir(keyFilePath(name)), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(keyFilePath(name), []byte(key), 0o600); err != nil {
			return fmt.Errorf("writing key file for %q: %w", name, err)
		}
	}
	return nil
}

// noKeyErr is the shared "key not found" error across backends.
func noKeyErr(name string, cause error) error {
	return fmt.Errorf("no key for %q: run `psw set-key %s`: %w", name, name, cause)
}

func keychainGet(name string) (string, error) {
	switch keychainBackend() {
	case "security":
		out, err := securityCmd("find-generic-password",
			"-s", keychainService, "-a", name, "-w")
		if err != nil {
			return "", noKeyErr(name, err)
		}
		return strings.TrimSpace(string(out)), nil
	case "secret-tool":
		out, err := secretToolCmd(nil, "lookup",
			"service", keychainService, "account", name)
		if err != nil {
			return "", noKeyErr(name, err)
		}
		return strings.TrimSpace(string(out)), nil
	default:
		data, err := os.ReadFile(keyFilePath(name))
		if err != nil {
			return "", noKeyErr(name, err)
		}
		return strings.TrimSpace(string(data)), nil
	}
}

func keychainDelete(name string) error {
	switch keychainBackend() {
	case "security":
		if _, err := securityCmd("delete-generic-password",
			"-s", keychainService, "-a", name); err != nil {
			return fmt.Errorf("deleting key for %q from Keychain: %w", name, err)
		}
	case "secret-tool":
		if _, err := secretToolCmd(nil, "clear",
			"service", keychainService, "account", name); err != nil {
			return fmt.Errorf("deleting key for %q from the secret store: %w", name, err)
		}
	default:
		if err := os.Remove(keyFilePath(name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("deleting key file for %q: %w", name, err)
		}
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
