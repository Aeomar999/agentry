#!/usr/bin/env bash
# Calls the running configuration's API with its page token (read from the log).
# usage: tools/measure/api.sh <cfg> <METHOD> </api/path> [json body]
set -eu; . tools/measure/env.sh
CFG=$1; METHOD=$2; P=$3; BODY=${4:-}
TOKEN=$(grep -o 'token=[A-Za-z0-9_-]*' $M/$CFG.log | head -1 | cut -d= -f2)
if [ -n "$BODY" ]; then
  curl -sS -X "$METHOD" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' --data "$BODY" "http://$ADDR$P"
else
  curl -sS -X "$METHOD" -H "Authorization: Bearer $TOKEN" "http://$ADDR$P"
fi
echo
