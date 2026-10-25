#!/usr/bin/env bash
# Starts one configuration's server on 127.0.0.1:7900, detached from this shell.
# usage: tools/measure/start.sh <cfg> <binary> [extra serve flags]
#   cfg     A | B | C: the data dir is <cfg>-data, the log <cfg>.log, the pid <cfg>.pid
#   binary  agentry-dev | agentry-214 (built into the measure folder)
# The log's first lines hold the page token; api.sh reads it from there.
set -eu; . tools/measure/env.sh
CFG=$1; BIN=$2; shift 2
if ss -ltn | grep -q ":${ADDR##*:} "; then echo "port ${ADDR##*:} is busy"; exit 1; fi
mkdir -p $M/$CFG-data
# Session sockets need a short path and must not share the live server's dir.
SESS=$(mktemp -d /tmp/agm-$CFG-XXXX)
# env -i: no CLAUDE_* or AGENTRY_* from the calling agent reaches the server; setsid: it outlives this shell.
setsid nohup env -i HOME="$HOME" PATH="$PATH" USER="${USER:-$(id -un)}" LANG="${LANG:-C.UTF-8}" TERM=dumb \
  AGENTRY_SESSIONS_DIR="$SESS" \
  $M/$BIN serve -addr $ADDR -data $M/$CFG-data -repo $M/scratch-repo -budget 0 -max-turns 0 "$@" \
  > $M/$CFG.log 2>&1 < /dev/null &
echo $! > $M/$CFG.pid
for i in $(seq 1 30); do grep -q 'token=' $M/$CFG.log 2>/dev/null && break; sleep 1; done
grep -q 'token=' $M/$CFG.log || { echo "no token in $M/$CFG.log"; tail -5 $M/$CFG.log; exit 1; }
echo "$CFG: $BIN serve $* pid $(cat $M/$CFG.pid), sessions $SESS"
