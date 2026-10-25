#!/usr/bin/env bash
# The fix round: in the task's worktree, on its branch, one commit that makes format_duration
# return '0' for zero (the docstring still says 0 -> '0s') and changes the zero test to expect '0'.
# The same defect every run. Prints the diff; the hash goes to <cfg>-plant.hash.
# usage: tools/measure/plant.sh <cfg> <task-id>
set -eu; . tools/measure/env.sh
CFG=$1; ID=$2
WT=$(tools/measure/api.sh $CFG GET /api/tasks/$ID | jq -r .worktree.path)
[ -d "$WT" ] || { echo "no worktree at $WT"; exit 1; }
python3 -I - "$WT" <<'PY'
import re, sys
wt = sys.argv[1]
p = wt + "/durations.py"
src = open(p).read()
m = re.search(r"^def format_duration\(.*\):\n", src, re.M)
if not m:
    sys.exit("no def format_duration in durations.py")
head, body = src[:m.end()], src[m.end():]
indent = re.match(r"[ \t]*", body).group(0) or "    "
doc = re.match(r'[ \t]*("""|\'\'\')(.*?)\1\n', body, re.S)
if doc:
    if "0 -> '0s'" not in doc.group(2) and '0 -> "0s"' not in doc.group(2):
        q = doc.group(1)
        body = body[:doc.end()-len(q)-1] + " 0 -> '0s'." + q + "\n" + body[doc.end():]
        doc = re.match(r'[ \t]*("""|\'\'\')(.*?)\1\n', body, re.S)
    cut = doc.end()
else:
    body = indent + "\"\"\"Format seconds as text, the inverse of parse_duration; 0 -> '0s'.\"\"\"\n" + body
    cut = len(body.split("\n", 1)[0]) + 1
body = body[:cut] + indent + "if seconds == 0:\n" + indent + "    return '0'\n" + body[cut:]
open(p, "w").write(head + body)
t = wt + "/test_durations.py"
tests = open(t).read()
lines = tests.split("\n")
changed = 0
for i, line in enumerate(lines):
    if "format_duration(0)" in line and re.search(r"""['"]0s['"]""", line):
        lines[i] = re.sub(r"""(['"])0s\1""", r"\g<1>0\1", line); changed += 1
if not changed:
    sys.exit("no test line with format_duration(0) and '0s' to change")
open(t, "w").write("\n".join(lines))
print("planted: zero returns '0'; %d test line(s) now expect '0'" % changed)
PY
(cd "$WT" && python3 -m unittest -q 2>&1 | tail -3)
git -C "$WT" --no-pager diff
git -C "$WT" -c user.name=engineer -c user.email=engineer@example.invalid commit -qam "fix: review round"
git -C "$WT" rev-parse --short HEAD | tee $M/$CFG-plant.hash
