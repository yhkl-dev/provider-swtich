package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseProvider(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []kv
		wantErr bool
	}{
		{
			name: "empty file",
			in:   "",
			want: []kv{},
		},
		{
			name: "comments and blank lines skipped",
			in:   "# comment\n\n  # indented comment\n",
			want: []kv{},
		},
		{
			name: "plain lines",
			in:   "ANTHROPIC_BASE_URL=https://example.com\nCLAUDE_CODE_EFFORT_LEVEL=max\n",
			want: []kv{
				{"ANTHROPIC_BASE_URL", "https://example.com"},
				{"CLAUDE_CODE_EFFORT_LEVEL", "max"},
			},
		},
		{
			name: "value contains equals and brackets",
			in:   "ANTHROPIC_DEFAULT_OPUS_MODEL=glm-5.2[1m]\nMIXED=a=b=c\n",
			want: []kv{
				{"ANTHROPIC_DEFAULT_OPUS_MODEL", "glm-5.2[1m]"},
				{"MIXED", "a=b=c"},
			},
		},
		{
			name:    "line without equals is an error",
			in:      "JUSTKEYWORD\n",
			wantErr: true,
		},
		{
			name: "crlf tolerated",
			in:   "A=1\r\n",
			want: []kv{{"A", "1"}},
		},
		{
			name: "trailing spaces preserved in value, key trimmed",
			in:   "  KEY =value  \n",
			want: []kv{{"KEY", "value"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseProvider(strings.NewReader(tt.in))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseProvider(%q) = %v, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseProvider(%q): %v", tt.in, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseProvider(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSaveProviderAndList(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PSW_CONFIG_DIR", dir)

	kvs := []kv{
		{"ANTHROPIC_BASE_URL", "https://example.com"},
		{"CLAUDE_CODE_EFFORT_LEVEL", "max"},
	}
	if err := saveProvider("relay-a", kvs); err != nil {
		t.Fatalf("saveProvider: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "providers", "relay-a"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := "ANTHROPIC_BASE_URL=https://example.com\nCLAUDE_CODE_EFFORT_LEVEL=max\n"
	if string(data) != want {
		t.Errorf("file = %q, want %q", data, want)
	}

	if err := saveProvider("official", nil); err != nil {
		t.Fatalf("saveProvider empty: %v", err)
	}

	got, err := listProviders()
	if err != nil {
		t.Fatalf("listProviders: %v", err)
	}
	wantNames := []string{"official", "relay-a"}
	if !reflect.DeepEqual(got, wantNames) {
		t.Errorf("listProviders = %v, want %v", got, wantNames)
	}
}

func TestConfigDirNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("PSW_CONFIG_DIR", "")
	if got, want := configDir(), filepath.Join(".config", "provider-switcher"); got != want {
		t.Errorf("configDir = %q, want %q", got, want)
	}
}

func TestProviderPath(t *testing.T) {
	t.Setenv("PSW_CONFIG_DIR", "/tmp/psw-test")
	if got, want := providerPath("zhipu"), "/tmp/psw-test/providers/zhipu"; got != want {
		t.Errorf("providerPath = %q, want %q", got, want)
	}
	if got, want := providersDir(), "/tmp/psw-test/providers"; got != want {
		t.Errorf("providersDir = %q, want %q", got, want)
	}
}

func TestValidateName(t *testing.T) {
	valid := []string{"official", "relay-a", "zhipu_maas"}
	for _, n := range valid {
		if err := validateName(n); err != nil {
			t.Errorf("validateName(%q) = %v, want nil", n, err)
		}
	}
	invalid := []string{"", ".", "..", "a/b", "with space"}
	for _, n := range invalid {
		if err := validateName(n); err == nil {
			t.Errorf("validateName(%q) = nil, want error", n)
		}
	}
}
