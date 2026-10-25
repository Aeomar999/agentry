#!/usr/bin/env bash
# Prints the numbers the report needs from a saved run: each reviewer turn's split, mode and
# whether its report names the planted zero defect, plus the task's totals.
# usage: tools/measure/summary.sh <cfg>
set -eu; . tools/measure/env.sh
CFG=$1
jq -r '"task \(.id) column=\(.column) turns=\(.turnList|length) tokensUsed=\(.tokensUsed) split=\(.tokensSplit)"' $M/$CFG-task.json
jq -r '.turnList[] | "\(.id) \(.agent) ended=\(.ended) tokens=\(.tokens) split=\(.split) fresh=\(.freshBecause // "-") from=\(.fromSession // "-") session=\(.engineSession // "-") forked=\(.forked // false)"' $M/$CFG-task.json
echo "--- reviewer reports (turn.ended)"
jq -r --arg rev $REVIEWER '.events[] | select(.type=="turn.ended" and .data.agent==$rev) | "## \(.correlation // .turn // "?")\n\(.data.report)\n"' $M/$CFG-events.json
