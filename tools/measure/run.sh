#!/usr/bin/env bash
# One measured run, in steps so no single command runs long; wait between them with wait.sh.
# usage: tools/measure/run.sh <cfg> task      posts the task; its id goes to <cfg>-task.id
#        tools/measure/wait.sh <cfg> <id> reviewer 1   (until the first review has ended)
#        tools/measure/run.sh <cfg> plant     commits the fix round with the planted defect (plant.sh)
#        tools/measure/run.sh <cfg> rereview  moves the task to review with the note
#        tools/measure/wait.sh <cfg> <id> reviewer 2   (until the re-review has ended)
#        tools/measure/run.sh <cfg> finish    marks it done, saves <cfg>-task.json and <cfg>-events.json
set -eu; . tools/measure/env.sh
CFG=$1; STEP=$2
ID=$(cat $M/$CFG-task.id 2>/dev/null || true)
case $STEP in
task)
  tools/measure/api.sh $CFG POST /api/tasks '{"title":"Add format_duration to durations.py","description":"Add format_duration(seconds) to durations.py, the inverse of parse_duration: 5400 -> '"'"'1h30m'"'"', 45 -> '"'"'45s'"'"', 0 -> '"'"'0s'"'"'. Add tests in test_durations.py. Done when python3 -m pytest passes (or python3 -m unittest if pytest is missing). Engineer builds, reviewer reviews, nothing else: no tester, no deployer, no push. When the reviewer approves, finish. Reviewer: a SHORT report, the verdict, then numbered findings with file:line, then the verdict again as the last line.","team":"Measure","kind":"story","budget":-1,"maxTurns":-1}' \
    | tee $M/$CFG-task-created.json | jq -r .stream > $M/$CFG-task.id
  echo "task $(cat $M/$CFG-task.id)";;
plant)
  tools/measure/plant.sh $CFG $ID;;
rereview)
  HASH=$(cat $M/$CFG-plant.hash)
  tools/measure/api.sh $CFG POST /api/tasks/$ID/direct "{\"to\":\"review\",\"note\":\"I committed the fix round on the branch myself ($HASH). Have the reviewer review the whole change again before anything else.\"}";;
finish)
  tools/measure/api.sh $CFG POST /api/tasks/$ID/direct '{"to":"done"}' | head -c 300; echo
  tools/measure/wait.sh $CFG $ID none || true
  tools/measure/api.sh $CFG GET /api/tasks/$ID > $M/$CFG-task.json
  tools/measure/api.sh $CFG GET "/api/tasks/$ID/events?limit=2000" > $M/$CFG-events.json
  echo "saved $M/$CFG-task.json and $M/$CFG-events.json"
  # The next run's task is task-1 again, so its branch in the scratch repo is renamed out of the way.
  git -C $M/scratch-repo branch -m agentry/$ID measure/$CFG-$ID && echo "branch kept as measure/$CFG-$ID";;
*) echo "unknown step $STEP"; exit 2;;
esac
