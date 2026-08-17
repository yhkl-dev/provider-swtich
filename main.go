package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const usageText = `psw - switch between model API providers

Usage:
  psw add <name> [--url URL]   create a provider (prompts for API key, stored in the secret store)
  psw rm <name>                remove a provider and its key
  psw edit <name>              edit a provider's env vars in $EDITOR
  psw set-key <name>           replace the API key in Keychain
  psw list                     list providers, marking the active one
  psw use <name>               switch active provider, print export statements
  psw current                  print the active provider
  psw env                      print export statements for the active provider
  psw init                     print the shell rc hook line

State lives in ~/.config/provider-switcher/ (override with PSW_CONFIG_DIR).
To apply in the current shell:  eval "$(psw use <name>)"
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches a command. It returns the process exit code and never
// touches os.Exit, so tests can drive it directly.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return 1
	}
	var err error
	switch args[0] {
	case "add":
		err = cmdAdd(args[1:], stdout, stderr)
	case "rm":
		err = cmdRm(args[1:], stdout, stderr)
	case "edit":
		err = cmdEdit(args[1:], stdout, stderr)
	case "set-key":
		err = cmdSetKey(args[1:], stdout, stderr)
	case "list":
		err = cmdList(args[1:], stdout, stderr)
	case "use":
		err = cmdUse(args[1:], stdout, stderr)
	case "current":
		err = cmdCurrent(args[1:], stdout, stderr)
	case "env":
		err = cmdEnv(args[1:], stdout, stderr)
	case "init":
		err = cmdInit(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usageText)
		return 0
	default:
		fmt.Fprintf(stderr, "psw: unknown command %q\n\n%s", args[0], usageText)
		return 1
	}
	if err != nil {
		fmt.Fprintf(stderr, "psw: %v\n", err)
		return 1
	}
	return 0
}

func cmdAdd(args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: psw add <name> [--url URL]")
	}
	name := args[0]
	var url string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--url":
			if i+1 >= len(args) {
				return fmt.Errorf("--url requires a value")
			}
			url = args[i+1]
			i++
		default:
			return fmt.Errorf("unknown flag %q", args[i])
		}
	}
	if err := validateName(name); err != nil {
		return err
	}
	if providerExists(name) {
		return fmt.Errorf("provider %q already exists", name)
	}
	key, err := promptSecret("API key for "+name+": ", stderr)
	if err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("empty API key, provider not added")
	}
	if err := keychainStore(name, key); err != nil {
		return err
	}
	warnFileBackend(name, stderr)
	var kvs []kv
	if url != "" {
		kvs = []kv{{"ANTHROPIC_BASE_URL", url}}
	}
	if err := saveProvider(name, kvs); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "added provider %q. Add env vars with `psw edit %s`.\n", name, name)
	return nil
}

func cmdRm(args []string, stdout, stderr io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: psw rm <name>")
	}
	name := args[0]
	if !providerExists(name) {
		return fmt.Errorf("unknown provider %q", name)
	}
	if active, err := getActive(); err == nil && active == name {
		return fmt.Errorf("provider %q is active: switch first with `psw use <name>`", name)
	}
	// The provider file is the source of truth; a missing Keychain entry
	// should not block removal.
	if err := keychainDelete(name); err != nil {
		fmt.Fprintf(stderr, "psw: warning: %v\n", err)
	}
	if err := os.Remove(providerPath(name)); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "removed provider %q\n", name)
	return nil
}

func cmdEdit(args []string, stdout, stderr io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: psw edit <name>")
	}
	name := args[0]
	if !providerExists(name) {
		return fmt.Errorf("unknown provider %q", name)
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		return fmt.Errorf("EDITOR is not set")
	}
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], providerPath(name))...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func cmdSetKey(args []string, stdout, stderr io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: psw set-key <name>")
	}
	name := args[0]
	if !providerExists(name) {
		return fmt.Errorf("unknown provider %q", name)
	}
	key, err := promptSecret("New API key for "+name+": ", stderr)
	if err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("empty API key, not changed")
	}
	if err := keychainStore(name, key); err != nil {
		return err
	}
	warnFileBackend(name, stderr)
	fmt.Fprintf(stdout, "updated key for provider %q\n", name)
	return nil
}

// warnFileBackend tells the user the key landed in a 0600 file instead of
// a system secret store, so they know why there is no Keychain entry.
func warnFileBackend(name string, stderr io.Writer) {
	if keychainBackend() == "file" {
		fmt.Fprintf(stderr, "psw: warning: no secret store (security/secret-tool) found; key stored in %s (0600)\n",
			keyFilePath(name))
	}
}

func cmdList(args []string, stdout, stderr io.Writer) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: psw list")
	}
	names, err := listProviders()
	if err != nil {
		return err
	}
	active, activeErr := getActive()
	for _, n := range names {
		mark := " "
		if activeErr == nil && n == active {
			mark = "*"
		}
		fmt.Fprintf(stdout, "%s %s\n", mark, n)
	}
	return nil
}

// renderFor prints export statements for one provider. The key is read
// live from Keychain; env vars come from the provider file.
func renderFor(name string, out io.Writer) error {
	key, err := keychainGet(name)
	if err != nil {
		return err
	}
	f, err := os.Open(providerPath(name))
	if err != nil {
		return err
	}
	defer f.Close()
	kvs, err := parseProvider(f)
	if err != nil {
		return err
	}
	fmt.Fprint(out, renderExports(key, kvs))
	return nil
}

func cmdUse(args []string, stdout, stderr io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: psw use <name>")
	}
	name := args[0]
	// Render before switching: a missing key or malformed file must not
	// leave the active provider pointing at something unusable.
	if err := renderFor(name, stdout); err != nil {
		return err
	}
	return setActive(name)
}

func cmdCurrent(args []string, stdout, stderr io.Writer) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: psw current")
	}
	name, err := getActive()
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, name)
	return nil
}

func cmdEnv(args []string, stdout, stderr io.Writer) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: psw env")
	}
	name, err := getActive()
	if err != nil {
		return err
	}
	return renderFor(name, stdout)
}

func cmdInit(args []string, stdout, stderr io.Writer) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: psw init")
	}
	// Add this line to ~/.zshrc so new shells pick up the last selection.
	fmt.Fprintln(stdout, `eval "$(psw env 2>/dev/null)"`)
	return nil
}
