#!/bin/sh
# Starts Agentry in its container. Everything worth keeping is in $HOME, a
# volume: Claude's sign-in, git's settings, the repos agents work on (under
# ~/work) and Agentry's log, worktrees and policy (~/.agentry).
set -eu

repo="${AGENTRY_REPO:-$HOME/work}"

# Who agents' commits are by, unless git already says.
if [ -n "${GIT_NAME:-}" ] && ! git config --global user.name >/dev/null; then
  git config --global user.name "$GIT_NAME"
fi
if [ -n "${GIT_EMAIL:-}" ] && ! git config --global user.email >/dev/null; then
  git config --global user.email "$GIT_EMAIL"
fi

# A repo to start in on a fresh volume: tasks need one. Clone your own
# repos next to it and name them on a task, or tell the desk.
if [ ! -d "$repo/.git" ]; then
  mkdir -p "$repo"
  git -C "$repo" init -q -b dev
  git -C "$repo" -c user.name=Agentry -c user.email=agentry@localhost commit -q --allow-empty -m "chore: start"
fi

exec agentry serve -addr 0.0.0.0:7800 -repo "$repo" \
  -public-url "${AGENTRY_PUBLIC_URL:-}" -budget "${AGENTRY_BUDGET:-5000000}"
