package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetAndGetActive(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PSW_CONFIG_DIR", dir)
	if err := saveProvider("relay-a", nil); err != nil {
		t.Fatal(err)
	}

	if _, err := getActive(); err == nil {
		t.Fatal("getActive with no active = nil error, want error")
	}

	if err := setActive("relay-a"); err != nil {
		t.Fatalf("setActive: %v", err)
	}
	got, err := getActive()
	if err != nil {
		t.Fatalf("getActive: %v", err)
	}
	if got != "relay-a" {
		t.Errorf("getActive = %q, want %q", got, "relay-a")
	}

	// The symlink points at the real provider file.
	target, err := os.Readlink(filepath.Join(dir, "active"))
	if err != nil {
		t.Fatal(err)
	}
	if target != filepath.Join("providers", "relay-a") {
		t.Errorf("symlink target = %q, want %q", target, "providers/relay-a")
	}
}

func TestSetActiveOverwrites(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PSW_CONFIG_DIR", dir)
	for _, n := range []string{"a", "b"} {
		if err := saveProvider(n, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := setActive("a"); err != nil {
		t.Fatal(err)
	}
	if err := setActive("b"); err != nil {
		t.Fatal(err)
	}
	got, err := getActive()
	if err != nil {
		t.Fatal(err)
	}
	if got != "b" {
		t.Errorf("getActive = %q, want %q", got, "b")
	}
}

func TestSetActiveUnknownProvider(t *testing.T) {
	t.Setenv("PSW_CONFIG_DIR", t.TempDir())
	if err := setActive("nope"); err == nil {
		t.Fatal("setActive(unknown) = nil, want error")
	}
}
