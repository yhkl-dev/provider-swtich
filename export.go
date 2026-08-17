package main

import (
	"fmt"
	"strings"
)

// shellQuote wraps v in single quotes, escaping embedded single quotes
// the only way POSIX shell allows: close quote, literal quote, reopen.
func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// renderExports renders export statements for the given API key and env pairs.
// The key is always exported as ANTHROPIC_API_KEY; pairs come from the
// provider file in file order.
func renderExports(apiKey string, kvs []kv) string {
	var b strings.Builder
	fmt.Fprintf(&b, "export ANTHROPIC_API_KEY=%s\n", shellQuote(apiKey))
	for _, e := range kvs {
		fmt.Fprintf(&b, "export %s=%s\n", e.key, shellQuote(e.value))
	}
	return b.String()
}
