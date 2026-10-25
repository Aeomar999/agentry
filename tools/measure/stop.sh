#!/usr/bin/env bash
# Stops a configuration's server and the agent processes it left behind; waits for the port.
# usage: tools/measure/stop.sh <cfg>
set -u; . tools/measure/env.sh
CFG=$1
[ -f $M/$CFG.pid ] && kill "$(cat $M/$CFG.pid)" 2>/dev/null
for i in $(seq 1 30); do ss -ltn | grep -q ":${ADDR##*:} " || break; sleep 1; done
# Session hosts and claude processes name the data dir in their arguments; never match this shell.
for p in $(pgrep -f -- "$M/$CFG-data"); do [ "$p" = "$$" ] || kill "$p" 2>/dev/null; done
sleep 1
ss -ltn | grep -q ":${ADDR##*:} " && { echo "port still busy"; exit 1; }
echo "$CFG stopped; port ${ADDR##*:} free"
