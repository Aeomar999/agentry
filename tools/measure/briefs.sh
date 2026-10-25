#!/usr/bin/env bash
# Copies the named turn briefs out of a run's task-1 worktree into <cfg>-briefs/, for the report.
# The server removes nothing from that worktree, so this works while it runs or after stop.sh.
# usage: tools/measure/briefs.sh <cfg> <turn-n>...     e.g. tools/measure/briefs.sh C turn-4 turn-10
set -eu; . tools/measure/env.sh
CFG=$1; shift
SRC=$M/$CFG-data/worktrees/task-1/.agentry
DST=$M/$CFG-briefs
mkdir -p $DST
for T in "$@"; do
  [ -f $SRC/$T.md ] || { echo "no $SRC/$T.md"; exit 1; }
  cp $SRC/$T.md $DST/$T.md
done
(cd $DST && for T in "$@"; do printf "%s: %s bytes, %s lines\n" $T.md $(wc -c < $T.md) $(wc -l < $T.md); done)
