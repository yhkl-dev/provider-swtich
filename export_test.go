package main

import "testing"

func TestShellQuote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"sk-abc123", "'sk-abc123'"},
		{"glm-5.2[1m]", "'glm-5.2[1m]'"},
		{"has space", "'has space'"},
		{"", "''"},
		{"a'b", `'a'\''b'`},
		{"$HOME", "'$HOME'"},
	}
	for _, tt := range tests {
		if got := shellQuote(tt.in); got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderExports(t *testing.T) {
	kvs := []kv{
		{"ANTHROPIC_BASE_URL", "https://token-plan.cn-beijing.maas.aliyuncs.com/apps/anthropic"},
		{"ANTHROPIC_DEFAULT_OPUS_MODEL", "glm-5.2[1m]"},
	}
	want := "export ANTHROPIC_API_KEY='sk-test'\n" +
		"export ANTHROPIC_BASE_URL='https://token-plan.cn-beijing.maas.aliyuncs.com/apps/anthropic'\n" +
		"export ANTHROPIC_DEFAULT_OPUS_MODEL='glm-5.2[1m]'\n"
	if got := renderExports("sk-test", kvs); got != want {
		t.Errorf("renderExports:\ngot  %q\nwant %q", got, want)
	}
}
