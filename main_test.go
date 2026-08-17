package main

import (
	"bufio"
	"strings"
	"testing"
)

// setupRun prepares the shared environment for run() tests.
func setupRun(t *testing.T) {
	t.Helper()
	t.Setenv("PSW_CONFIG_DIR", t.TempDir())
	fakeSecurityShim(t)
	t.Setenv("FAKE_KEY", "sk-test")
	oldReader := stdinReader
	stdinReader = bufio.NewReader(strings.NewReader(strings.Repeat("sk-test\n", 5)))
	t.Cleanup(func() { stdinReader = oldReader })
}

func TestRunAddUseEnvCurrentListRm(t *testing.T) {
	setupRun(t)
	var out, errb strings.Builder

	if code := run([]string{"add", "relay-a", "--url", "https://api.example.com"}, &out, &errb); code != 0 {
		t.Fatalf("add = %d, stderr: %s", code, errb.String())
	}

	// Duplicate add fails.
	if code := run([]string{"add", "relay-a"}, &out, &errb); code == 0 {
		t.Fatal("duplicate add = 0, want non-zero")
	}

	// use prints exports and marks active.
	out.Reset()
	errb.Reset()
	if code := run([]string{"use", "relay-a"}, &out, &errb); code != 0 {
		t.Fatalf("use = %d, stderr: %s", code, errb.String())
	}
	for _, want := range []string{
		"export ANTHROPIC_API_KEY='sk-test'",
		"export ANTHROPIC_BASE_URL='https://api.example.com'",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("use output missing %q\ngot:\n%s", want, out.String())
		}
	}

	// current
	out.Reset()
	if code := run([]string{"current"}, &out, &errb); code != 0 {
		t.Fatalf("current = %d, stderr: %s", code, errb.String())
	}
	if got := strings.TrimSpace(out.String()); got != "relay-a" {
		t.Errorf("current = %q, want relay-a", got)
	}

	// env prints same exports.
	out.Reset()
	if code := run([]string{"env"}, &out, &errb); code != 0 {
		t.Fatalf("env = %d, stderr: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "export ANTHROPIC_API_KEY='sk-test'") {
		t.Errorf("env output missing key export\ngot:\n%s", out.String())
	}

	// list marks active.
	out.Reset()
	if code := run([]string{"list"}, &out, &errb); code != 0 {
		t.Fatalf("list = %d, stderr: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "* relay-a") {
		t.Errorf("list output missing active mark\ngot:\n%s", out.String())
	}

	// rm refuses to remove the active provider.
	if code := run([]string{"rm", "relay-a"}, &out, &errb); code == 0 {
		t.Fatal("rm active = 0, want non-zero")
	}

	// rm unknown provider fails.
	if code := run([]string{"rm", "nope"}, &out, &errb); code == 0 {
		t.Fatal("rm unknown = 0, want non-zero")
	}
}

func TestRunUseSwitchesAndRmWorksAfterSwitch(t *testing.T) {
	setupRun(t)
	var out, errb strings.Builder

	if code := run([]string{"add", "a"}, &out, &errb); code != 0 {
		t.Fatalf("add a: %s", errb.String())
	}
	if code := run([]string{"add", "b"}, &out, &errb); code != 0 {
		t.Fatalf("add b: %s", errb.String())
	}
	if code := run([]string{"use", "a"}, &out, &errb); code != 0 {
		t.Fatalf("use a: %s", errb.String())
	}
	if code := run([]string{"use", "b"}, &out, &errb); code != 0 {
		t.Fatalf("use b: %s", errb.String())
	}
	out.Reset()
	if code := run([]string{"current"}, &out, &errb); code != 0 {
		t.Fatalf("current: %s", errb.String())
	}
	if got := strings.TrimSpace(out.String()); got != "b" {
		t.Errorf("current = %q, want b", got)
	}

	// a is no longer active, removal succeeds.
	if code := run([]string{"rm", "a"}, &out, &errb); code != 0 {
		t.Fatalf("rm a: %s", errb.String())
	}
}

func TestRunUseUnknownProvider(t *testing.T) {
	setupRun(t)
	var out, errb strings.Builder
	if code := run([]string{"use", "nope"}, &out, &errb); code == 0 {
		t.Fatal("use unknown = 0, want non-zero")
	}
}

func TestRunEnvNoActive(t *testing.T) {
	setupRun(t)
	var out, errb strings.Builder
	if code := run([]string{"env"}, &out, &errb); code == 0 {
		t.Fatal("env with no active = 0, want non-zero")
	}
	if out.String() != "" {
		t.Errorf("env stdout = %q, want empty", out.String())
	}
}

func TestRunInitPrintsRcHook(t *testing.T) {
	setupRun(t)
	var out, errb strings.Builder
	if code := run([]string{"init"}, &out, &errb); code != 0 {
		t.Fatalf("init: %s", errb.String())
	}
	if !strings.Contains(out.String(), `eval "$(psw env 2>/dev/null)"`) {
		t.Errorf("init output missing rc hook\ngot:\n%s", out.String())
	}
}

func TestRunEditUsesEditor(t *testing.T) {
	setupRun(t)
	var out, errb strings.Builder
	if code := run([]string{"add", "a"}, &out, &errb); code != 0 {
		t.Fatalf("add: %s", errb.String())
	}
	t.Setenv("EDITOR", "cat")
	out.Reset()
	if code := run([]string{"edit", "a"}, &out, &errb); code != 0 {
		t.Fatalf("edit: %s", errb.String())
	}
	// cat prints the (empty) provider file; just verify it ran without error.
	_ = out.String()
}

func TestRunAddStoreFailureLeavesNoProvider(t *testing.T) {
	setupRun(t)
	t.Setenv("FAKE_STORE_FAIL", "1")
	var out, errb strings.Builder
	if code := run([]string{"add", "a"}, &out, &errb); code == 0 {
		t.Fatal("add with failing keychain = 0, want non-zero")
	}
	if providerExists("a") {
		t.Fatal("provider file exists after failed keychain store")
	}
}

func TestRunUseMissingKeyDoesNotSwitch(t *testing.T) {
	setupRun(t)
	var out, errb strings.Builder
	if code := run([]string{"add", "a"}, &out, &errb); code != 0 {
		t.Fatalf("add: %s", errb.String())
	}
	t.Setenv("FAKE_KEY", "")
	if code := run([]string{"use", "a"}, &out, &errb); code == 0 {
		t.Fatal("use with missing key = 0, want non-zero")
	}
	// Active must be untouched: render happens before the symlink switch.
	if code := run([]string{"current"}, &out, &errb); code == 0 {
		t.Fatal("current = 0 after failed use, want non-zero")
	}
}

func TestRunUsageErrors(t *testing.T) {
	setupRun(t)
	cases := [][]string{
		{"add"},
		{"add", "a", "--url"},
		{"add", "a", "--bogus", "x"},
		{"rm"},
		{"rm", "a", "b"},
		{"edit"},
		{"set-key"},
		{"use"},
		{"current", "x"},
		{"env", "x"},
		{"init", "x"},
		{"frobnicate"},
	}
	for _, args := range cases {
		var out, errb strings.Builder
		if code := run(args, &out, &errb); code == 0 {
			t.Errorf("run(%v) = 0, want non-zero", args)
		}
	}
	var out, errb strings.Builder
	if code := run([]string{"help"}, &out, &errb); code != 0 {
		t.Errorf("help = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "psw use <name>") {
		t.Errorf("help output missing usage\ngot:\n%s", out.String())
	}
}

func TestRunAddEmptyKey(t *testing.T) {
	setupRun(t)
	oldReader := stdinReader
	stdinReader = bufio.NewReader(strings.NewReader("\n"))
	t.Cleanup(func() { stdinReader = oldReader })
	var out, errb strings.Builder
	if code := run([]string{"add", "a"}, &out, &errb); code == 0 {
		t.Fatal("add with empty key = 0, want non-zero")
	}
	if providerExists("a") {
		t.Fatal("provider file exists after empty key")
	}
}

func TestRunEditUnknownAndNoEditor(t *testing.T) {
	setupRun(t)
	var out, errb strings.Builder
	if code := run([]string{"edit", "nope"}, &out, &errb); code == 0 {
		t.Fatal("edit unknown = 0, want non-zero")
	}
	if code := run([]string{"add", "a"}, &out, &errb); code != 0 {
		t.Fatalf("add: %s", errb.String())
	}
	t.Setenv("EDITOR", "")
	if code := run([]string{"edit", "a"}, &out, &errb); code == 0 {
		t.Fatal("edit without EDITOR = 0, want non-zero")
	}
}

func TestRunSetKeyAndNoArgs(t *testing.T) {
	setupRun(t)
	var out, errb strings.Builder
	if code := run([]string{"add", "a"}, &out, &errb); code != 0 {
		t.Fatalf("add: %s", errb.String())
	}

	oldReader := stdinReader
	stdinReader = bufio.NewReader(strings.NewReader("sk-new\n"))
	if code := run([]string{"set-key", "a"}, &out, &errb); code != 0 {
		t.Fatalf("set-key: %s", errb.String())
	}
	stdinReader = oldReader

	if code := run(nil, &out, &errb); code == 0 {
		t.Fatal("no args = 0, want non-zero")
	}
}
