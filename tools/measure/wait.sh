#!/usr/bin/env bash
# Waits (at most 540 s, polling every 30 s) until an agent's n-th turn on the task has ended,
# or, with agent "none", until no turn is open. Exit 0 when reached, 3 when still waiting.
# usage: tools/measure/wait.sh <cfg> <task-id> <agent|none> [n=1]
set -eu; . tools/measure/env.sh
CFG=$1; TASK=$2; AGENT=$3; N=${4:-1}
for i in $(seq 1 18); do
  J=$(tools/measure/api.sh $CFG GET /api/tasks/$TASK)
  OPEN=$(echo "$J" | jq '[.turnList[] | select(.ended|not)] | length')
  LAST=$(echo "$J" | jq -r '.turnList | last | "\(.id) \(.agent) ended=\(.ended) tokens=\(.tokens)"')
  echo "$(date +%T) column=$(echo "$J" | jq -r .column) open=$OPEN last: $LAST"
  if [ "$AGENT" = none ]; then
    [ "$OPEN" = 0 ] && exit 0
  else
    DONE=$(echo "$J" | jq --arg a "$AGENT" --argjson n "$N" '[.turnList[] | select(.agent==$a)] | length >= $n and .[$n-1].ended')
    [ "$DONE" = true ] && exit 0
  fi
  [ $i -lt 18 ] && sleep 30
done
exit 3
