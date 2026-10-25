#!/usr/bin/env bash
# Adds the "Measure" team: the Software template's coordinator, engineer and reviewer, so no
# tester or deployer turns run. The reviewer keeps column "review"; resume is left "".
# usage: tools/measure/team.sh <cfg>
set -eu; . tools/measure/env.sh
CFG=$1
BODY=$(tools/measure/api.sh $CFG GET /api/teams/templates | jq -c '
  [.[] | select(.team=="Software" or (.id|ascii_downcase|test("software")))][0] as $t
  | {team: "Measure", copiedFrom: $t.id,
     agents: [$t.agents[] | select(.coordinates or .name=="engineer" or .name=="reviewer")
              | {name, description, prompt, engine, model, profile, color, coordinates,
                 column: (if .name=="reviewer" then "review" else .column end), resume: ""}]}')
echo "$BODY" | jq -c '{team, copiedFrom, agents: [.agents[] | {name, coordinates, column}]}'
tools/measure/api.sh $CFG POST /api/teams "$BODY"
