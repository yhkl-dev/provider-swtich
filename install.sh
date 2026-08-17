#!/bin/sh
# Install psw: build, install to PREFIX/bin, add shell wrapper to rc file.
# Override with: PREFIX=~/.local RC=~/.bashrc ./install.sh
set -eu

PREFIX="${PREFIX:-$HOME/.local}"
BIN="$PREFIX/bin"
mkdir -p "$BIN"

go build -o psw .
install -m 755 psw "$BIN/psw"
rm -f psw
echo "installed psw to $BIN/psw"

# On Linux, note the storage backend psw will use: secret-tool when
# present, a 0600 file otherwise. macOS always has Keychain (`security`).
case "$(uname -s)" in
    Linux)
        if ! command -v secret-tool >/dev/null 2>&1; then
            echo "note: secret-tool not found; psw will store keys in a 0600 file"
        fi
        ;;
esac

RC="${RC:-$HOME/.zshrc}"
if [ -f "$RC" ] && grep -q '# psw:' "$RC"; then
    echo "shell wrapper already in $RC"
    exit 0
fi

cat >> "$RC" <<'EOF'

# psw: switch provider with `psw use <name>` applied to the current shell
psw() {
  if [[ "$1" == "use" ]]; then
    eval "$(command psw use "$2")"
  else
    command psw "$@"
  fi
}
EOF
echo "added psw shell wrapper to $RC — run: source $RC"
