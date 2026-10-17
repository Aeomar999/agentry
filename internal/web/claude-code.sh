#!/bin/sh
# Gives Claude Code on this machine Agentry's tools, for every repo: installs
# the agentry command, connects it to the server with a one-time code, and
# adds it to Claude Code with its status line. Run again with a new command
# from the Integrations page, it updates agentry and connects afresh.
#
#   AGENTRY_BIN  where agentry goes (default: ~/.local/bin)
set -eu

server={{SERVER}}
code={{CODE}}
bin="${AGENTRY_BIN:-$HOME/.local/bin}"

say() { printf '%s\n' "$*"; }

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $(uname -m) in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) arch=$(uname -m) ;;
esac

say "Installing agentry in $bin"
mkdir -p "$bin"
if ! curl -fsSL "$server/integrations/agentry?code=$code&os=$os&arch=$arch" -o "$bin/agentry.part"; then
  rm -f "$bin/agentry.part"
  say "The server has no agentry for $os/$arch." >&2
  exit 1
fi
chmod +x "$bin/agentry.part"
mv "$bin/agentry.part" "$bin/agentry"

"$bin/agentry" mcp connect "$server" "$code"

if command -v claude >/dev/null 2>&1; then
  "$bin/agentry" mcp install
else
  say "Claude Code isn't installed here. Install it, then run: $bin/agentry mcp install"
fi

# Put agentry on the PATH of new shells, if it isn't already.
case ":$PATH:" in
  *":$bin:"*) ;;
  *)
    case "${SHELL:-}" in
      */zsh) rc="$HOME/.zshrc" ;;
      */bash) [ "$os" = darwin ] && rc="$HOME/.bash_profile" || rc="$HOME/.bashrc" ;;
      *) rc="$HOME/.profile" ;;
    esac
    if ! grep -qs 'added by the Agentry install' "$rc"; then
      printf '\nexport PATH="%s:$PATH" # added by the Agentry install\n' "$bin" >>"$rc"
    fi
    say "Added $bin to your PATH in $rc; open a new terminal to use agentry."
    ;;
esac
say ""
say "Done. Start claude in any repo and ask it for your Agentry board."
