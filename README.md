# psw

Same model, multiple providers — switch API key and env vars in one command.

## Install

```sh
go build -o /usr/local/bin/psw .
psw init    # prints the one line to add to ~/.zshrc
```

The zshrc line makes new shells load your last selection automatically.

## Usage

```sh
psw add zhipu-maas --url https://token-plan.cn-beijing.maas.aliyuncs.com/apps/anthropic
# prompts for the API key, stored in macOS Keychain (service: provider-switcher)

psw edit zhipu-maas          # opens $EDITOR on the provider's env vars
# add lines like:
#   ANTHROPIC_DEFAULT_OPUS_MODEL=glm-5.2[1m]
#   CLAUDE_CODE_EFFORT_LEVEL=max

eval "$(psw use zhipu-maas)"  # switch now, exports into the current shell

psw list                     # * marks the active provider
psw current
psw env                      # exports for the active provider
psw set-key zhipu-maas       # replace the Keychain key
psw rm zhipu-maas            # refuses while active
```

## Layout

- `~/.config/provider-switcher/providers/<name>` — one file per provider,
  `KEY=VALUE` lines, `#` comments. The API key is never written here.
- `~/.config/provider-switcher/active` — symlink to the active provider.
- Keychain — service `provider-switcher`, account = provider name.
- `PSW_CONFIG_DIR` overrides the state directory (tests, containers).

All Go stdlib. Tests: `go test -race ./...`.
