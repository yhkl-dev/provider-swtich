package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSecurityShim puts fake `security` and `stty` binaries on PATH.
// `security find-generic-password` prints $FAKE_KEY and exits 44 when
// FAKE_KEY is unset; every invocation is logged to $FAKE_LOG.
func fakeSecurityShim(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log")
	script := `#!/bin/sh
echo "$@" >> "$FAKE_LOG"
if [ "$1" = find-generic-password ]; then
	if [ -n "$FAKE_KEY" ]; then
		echo "$FAKE_KEY"
		exit 0
	fi
	exit 44
fi
if [ "$1" = add-generic-password ] && [ -n "$FAKE_STORE_FAIL" ]; then
	exit 1
fi
exit 0
`
	writeExec(t, filepath.Join(dir, "security"), script)
	writeExec(t, filepath.Join(dir, "stty"),
		"#!/bin/sh\necho \"$@\" >> \"$FAKE_LOG\"\n[ -z \"$FAKE_STTY_FAIL\" ] || exit 1\nexit 0\n")
	t.Setenv("FAKE_LOG", logFile)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return logFile
}

func writeExec(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestKeychainStoreGetDelete(t *testing.T) {
	logFile := fakeSecurityShim(t)
	t.Setenv("FAKE_KEY", "sk-secret")

	if err := keychainStore("relay-a", "sk-secret"); err != nil {
		t.Fatalf("keychainStore: %v", err)
	}
	got, err := keychainGet("relay-a")
	if err != nil {
		t.Fatalf("keychainGet: %v", err)
	}
	if got != "sk-secret" {
		t.Errorf("keychainGet = %q, want %q", got, "sk-secret")
	}
	if err := keychainDelete("relay-a"); err != nil {
		t.Fatalf("keychainDelete: %v", err)
	}

	log := readLog(t, logFile)
	for _, want := range []string{
		"add-generic-password -s provider-switcher -a relay-a -w sk-secret -U",
		"find-generic-password -s provider-switcher -a relay-a -w",
		"delete-generic-password -s provider-switcher -a relay-a",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("log missing %q\ngot log:\n%s", want, log)
		}
	}
}

func TestKeychainGetMissing(t *testing.T) {
	fakeSecurityShim(t)
	if _, err := keychainGet("nope"); err == nil {
		t.Fatal("keychainGet(missing) = nil, want error")
	}
}

func TestPromptSecret(t *testing.T) {
	fakeSecurityShim(t)
	var out strings.Builder
	oldReader := stdinReader
	stdinReader = bufio.NewReader(strings.NewReader("sk-from-stdin\n"))
	t.Cleanup(func() { stdinReader = oldReader })

	got, err := promptSecret("API key: ", &out)
	if err != nil {
		t.Fatalf("promptSecret: %v", err)
	}
	if got != "sk-from-stdin" {
		t.Errorf("promptSecret = %q, want %q", got, "sk-from-stdin")
	}
	if !strings.Contains(out.String(), "API key: ") {
		t.Errorf("output %q missing prompt", out.String())
	}
}

func TestPromptSecretRunsSttyWhenTerminal(t *testing.T) {
	logFile := fakeSecurityShim(t)
	// /dev/null is a char device, so os.Stdin.Stat() reports a terminal.
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = oldStdin; f.Close() })

	oldReader := stdinReader
	stdinReader = bufio.NewReader(strings.NewReader("k\n"))
	t.Cleanup(func() { stdinReader = oldReader })

	var out strings.Builder
	if _, err := promptSecret("k: ", &out); err != nil {
		t.Fatalf("promptSecret: %v", err)
	}
	log := readLog(t, logFile)
	if !strings.Contains(log, "-echo") || !strings.Contains(log, "echo") {
		t.Errorf("stty not invoked, log:\n%s", log)
	}
}

func TestPromptSecretSttyFailure(t *testing.T) {
	fakeSecurityShim(t)
	t.Setenv("FAKE_STTY_FAIL", "1")
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = oldStdin; f.Close() })

	oldReader := stdinReader
	stdinReader = bufio.NewReader(strings.NewReader("k\n"))
	t.Cleanup(func() { stdinReader = oldReader })

	var out strings.Builder
	if _, err := promptSecret("k: ", &out); err == nil {
		t.Fatal("promptSecret with failing stty = nil, want error")
	}
}

func TestPromptSecretStripsCR(t *testing.T) {
	fakeSecurityShim(t)
	var out strings.Builder
	oldReader := stdinReader
	stdinReader = bufio.NewReader(strings.NewReader("key\r\n"))
	t.Cleanup(func() { stdinReader = oldReader })

	got, err := promptSecret("k: ", &out)
	if err != nil {
		t.Fatalf("promptSecret: %v", err)
	}
	if got != "key" {
		t.Errorf("promptSecret = %q, want %q", got, "key")
	}
}
