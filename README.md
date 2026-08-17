# psw

Same model, multiple providers — switch API key and env vars in one command.

[![CI](https://github.com/yhkl-dev/provider-swtich/actions/workflows/ci.yml/badge.svg)](https://github.com/yhkl-dev/provider-swtich/actions/workflows/ci.yml)

API keys live in the macOS Keychain; per-provider env vars in plain files under
`~/.config/provider-switcher/`. `psw use <name>` switches the active provider
and prints `export` statements for the current shell.

## Install

Requires Go and macOS (Keychain via the `security` CLI).

```sh
./install.sh
```

Builds `psw`, installs it to `~/.local/bin` (override with `PREFIX=...`), and
adds a shell wrapper to `~/.zshrc` so `psw use` applies to the current shell
directly. Then:

```sh
source ~/.zshrc
```

The wrapper also makes new shells load the last selection automatically.

Alternative: `make install PREFIX=~/.local` installs the binary only; add the
wrapper manually:

```sh
psw() {
  if [[ "$1" == "use" ]]; then
    eval "$(command psw use "$2")"
  else
    command psw "$@"
  fi
}
```

## Usage

```sh
psw add zhipu-maas --url https://token-plan.cn-beijing.maas.aliyuncs.com/apps/anthropic
# prompts for the API key, stored in macOS Keychain (service: provider-switcher)

psw edit zhipu-maas          # opens $EDITOR on the provider's env vars
# add lines like:
#   ANTHROPIC_DEFAULT_OPUS_MODEL=glm-5.2[1m]
#   CLAUDE_CODE_EFFORT_LEVEL=max

psw use zhipu-maas           # switch now — env applies to the current shell
psw list                     # * marks the active provider
psw current
psw env                      # exports for the active provider
psw set-key zhipu-maas       # replace the Keychain key
psw rm zhipu-maas            # refuses while active
```

Provider files are `KEY=VALUE` lines (no `export` prefix); lines starting with
`#` are comments and are skipped.

## Layout

- `~/.config/provider-switcher/providers/<name>` — one file per provider,
  `KEY=VALUE` lines, `#` comments. The API key is never written here.
- `~/.config/provider-switcher/active` — symlink to the active provider.
- Keychain — service `provider-switcher`, account = provider name.
- `PSW_CONFIG_DIR` overrides the state directory (tests, containers).

## Development

All Go stdlib. CI runs `go vet`, `go test -race -cover`, and a build on every
push and PR.

```sh
make check     # fmt + vet + race tests
make build
make install PREFIX=~/.local
```

## License

MIT
