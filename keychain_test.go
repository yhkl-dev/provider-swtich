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

// fakeSecretToolShim puts a fake `secret-tool` on PATH (and nothing else,
// so a host `security` binary cannot win the backend selection). `store`
// reads the key from stdin into $FAKE_STDIN_LOG; `lookup` prints $FAKE_KEY
// and exits 44 when FAKE_KEY is unset; every invocation is logged to
// $FAKE_LOG.
func fakeSecretToolShim(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log")
	stdinLog := filepath.Join(dir, "stdin")
	script := `#!/bin/sh
echo "$@" >> "$FAKE_LOG"
case "$1" in
  store)
    [ -z "$FAKE_STORE_FAIL" ] || exit 1
    IFS= read -r l
    echo "$l" >> "$FAKE_STDIN_LOG"
    exit 0
    ;;
  lookup)
    if [ -n "$FAKE_KEY" ]; then
      echo "$FAKE_KEY"
      exit 0
    fi
    exit 44
    ;;
esac
exit 0
`
	writeExec(t, filepath.Join(dir, "secret-tool"), script)
	t.Setenv("FAKE_LOG", logFile)
	t.Setenv("FAKE_STDIN_LOG", stdinLog)
	t.Setenv("PATH", dir)
	return logFile
}

func TestSecretToolStoreGetDelete(t *testing.T) {
	logFile := fakeSecretToolShim(t)
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
		"store --label psw: relay-a service provider-switcher account relay-a",
		"lookup service provider-switcher account relay-a",
		"clear service provider-switcher account relay-a",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("log missing %q\ngot log:\n%s", want, log)
		}
	}
	// The key travels on stdin, never in argv.
	if strings.Contains(log, "sk-secret") {
		t.Errorf("key appeared in argv log:\n%s", log)
	}
	stdinLog, err := os.ReadFile(os.Getenv("FAKE_STDIN_LOG"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stdinLog) != "sk-secret\n" {
		t.Errorf("stdin log = %q, want %q", stdinLog, "sk-secret\n")
	}
}

func TestSecretToolGetMissing(t *testing.T) {
	fakeSecretToolShim(t)
	if _, err := keychainGet("nope"); err == nil {
		t.Fatal("keychainGet(missing) = nil, want error")
	}
}

// fileBackendEnv points the backend selection at the 0600-file fallback:
// an empty PATH dir (no security, no secret-tool) plus an isolated config
// dir.
func fileBackendEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("PSW_CONFIG_DIR", t.TempDir())
}

func TestFileBackendStoreGetDelete(t *testing.T) {
	fileBackendEnv(t)
	if got := keychainBackend(); got != "file" {
		t.Fatalf("keychainBackend = %q, want file", got)
	}

	if err := keychainStore("relay-a", "sk-secret"); err != nil {
		t.Fatalf("keychainStore: %v", err)
	}
	path := keyFilePath("relay-a")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("key file mode = %o, want 600", fi.Mode().Perm())
	}
	dirFi, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat keys dir: %v", err)
	}
	if dirFi.Mode().Perm() != 0o700 {
		t.Errorf("keys dir mode = %o, want 700", dirFi.Mode().Perm())
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
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("key file still present after delete: %v", err)
	}
}

func TestFileBackendGetMissing(t *testing.T) {
	fileBackendEnv(t)
	if _, err := keychainGet("nope"); err == nil {
		t.Fatal("keychainGet(missing) = nil, want error")
	}
}

func TestKeychainBackendPrecedence(t *testing.T) {
	shim := "#!/bin/sh\nexit 0\n"
	cases := []struct {
		name string
		bins []string
		want string
	}{
		{"security wins over secret-tool", []string{"security", "secret-tool"}, "security"},
		{"secret-tool when no security", []string{"secret-tool"}, "secret-tool"},
		{"file when neither", nil, "file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, b := range tc.bins {
				writeExec(t, filepath.Join(dir, b), shim)
			}
			t.Setenv("PATH", dir)
			if got := keychainBackend(); got != tc.want {
				t.Errorf("keychainBackend = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSecurityStoreErrorRedactsKey(t *testing.T) {
	fakeSecurityShim(t)
	t.Setenv("FAKE_STORE_FAIL", "1")
	err := keychainStore("a", "sk-super-secret")
	if err == nil {
		t.Fatal("keychainStore = nil, want error")
	}
	if strings.Contains(err.Error(), "sk-super-secret") {
		t.Errorf("error leaks the key: %v", err)
	}
	if !strings.Contains(err.Error(), "-w ***") {
		t.Errorf("error missing redacted arg: %v", err)
	}
}

func TestRunAddFileBackendWarns(t *testing.T) {
	dir := t.TempDir()
	writeExec(t, filepath.Join(dir, "stty"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", dir)
	t.Setenv("PSW_CONFIG_DIR", t.TempDir())
	oldReader := stdinReader
	stdinReader = bufio.NewReader(strings.NewReader("sk-test\n"))
	t.Cleanup(func() { stdinReader = oldReader })

	var out, errb strings.Builder
	if code := run([]string{"add", "a"}, &out, &errb); code != 0 {
		t.Fatalf("add = %d, stderr: %s", code, errb.String())
	}
	if !strings.Contains(errb.String(), "0600") {
		t.Errorf("stderr missing file-backend warning\ngot:\n%s", errb.String())
	}
	fi, err := os.Stat(keyFilePath("a"))
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("key file mode = %o, want 600", fi.Mode().Perm())
	}
}
