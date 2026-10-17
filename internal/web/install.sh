#!/bin/sh
# Makes this machine an Agentry worker: installs the agentry command, joins
# the server with a one-time code, and starts the worker in the background.
# Run again on a machine that has joined, it updates agentry and restarts.
#
#   AGENTRY_WORKER_NAME  what to call this machine (default: its host name)
#   AGENTRY_BIN          where agentry goes (default: ~/.local/bin)
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
if ! curl -fsSL "$server/worker/agentry?code=$code&os=$os&arch=$arch" -o "$bin/agentry.part"; then
  rm -f "$bin/agentry.part"
  say "The server has no agentry for $os/$arch." >&2
  exit 1
fi
chmod +x "$bin/agentry.part"
mv "$bin/agentry.part" "$bin/agentry"

for tool in git tmux claude; do
  command -v "$tool" >/dev/null 2>&1 || say "Note: $tool isn't installed here; agents need it."
done

# Joins, or, if this machine has joined already, leaves the code unused.
"$bin/agentry" worker join ${AGENTRY_WORKER_NAME:+-name "$AGENTRY_WORKER_NAME"} "$server" "$code"

# A worker started by hand before (as older installs did) would run beside
# the service; it goes first.
pkill -f "$bin/agentry worker" 2>/dev/null || true
"$bin/agentry" worker start

# Put agentry on the PATH of new shells, if it isn't already.
case ":$PATH:" in
  *":$bin:"*) ;;
  *)
    case "${SHELL:-}" in
      */zsh) rc="$HOME/.zshrc" ;;
      */bash) [ "$os" = darwin ] && rc="$HOME/.bash_profile" || rc="$HOME/.bashrc" ;;
      *) rc="$HOME/.profile" ;;
    esac
    if ! grep -qs 'added by the Agentry worker install' "$rc"; then
      printf '\nexport PATH="%s:$PATH" # added by the Agentry worker install\n' "$bin" >>"$rc"
    fi
    say "Added $bin to your PATH in $rc; open a new terminal to use agentry."
    ;;
esac
say ""
say "Manage it with: agentry worker status | logs | stop | start | update | leave"
