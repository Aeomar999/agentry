import sys
from flat import Figure

OUT = sys.argv[1]

def save(f, name):
    open(f"{OUT}/{name}.svg", "w").write(f.svg())

# 1. Getting started ---------------------------------------------------------
f = Figure(1200, 790, "Getting started", "Agentry",
           "Unzip it, double-click Start Agentry, and it opens in your browser. You need git and Claude Code.")
systems = [
    (48, "Mac", "agentry-...-mac-apple-silicon.zip", "Start Agentry.command", ["right-click it, choose Open,", "then Open again"]),
    (424, "Windows", "agentry-...-windows-x64.zip", "Start Agentry.cmd", ["More info, then Run anyway", "(it isn't signed yet)"]),
    (800, "Linux", "agentry-...-linux-x64.zip", "start-agentry.sh", ["if it won't run:", "chmod +x the scripts"]),
]
for x, name, zip_name, start, first in systems:
    f.panel(x, 150, 352, 312, name)
    f.number(x + 32, 222, 1)
    f.card(x + 56, 190, 276, 64, "Download the zip", zip_name, "blue")
    f.number(x + 32, 308, 2)
    f.card(x + 56, 276, 276, 64, "Unzip, double-click", start, "blue")
    f.number(x + 32, 400, 3, "amber")
    f.card(x + 56, 362, 276, 76, "The first time", first, "amber")
    f.arrow(f"M{x + 194},258 V272", "muted", width=1.5)
    f.arrow(f"M{x + 194},344 V358", "muted", width=1.5)
f.arrow("M224,464 V488 H380 V524", "blue")
f.arrow("M600,464 V524", "blue")
f.arrow("M976,464 V488 H820 V524", "blue")
f.card(300, 528, 600, 84, "Agentry opens in your browser, signed in",
       ["It keeps running in the background. Run Start Agentry again", "to reopen the page; Stop Agentry stops it."], "blue")
f.rule(642, "Then, from the page")
f.card(48, 690, 352, 76, "Add machines", ["Workers: one command to run there,", "for its system. Agents can run on it."], "green")
f.card(424, 690, 352, 76, "Connect Claude Code", ["Integrations: one command, then", "plain claude in any repo has Agentry."], "blue")
f.card(800, 690, 352, 76, "Hand it work", ["New task, or tell Claude at the", "desk (Ctrl+J) what you want."], "clay")
save(f, "getting-started")

# 2. How it fits together ----------------------------------------------------
f = Figure(1200, 690, "How Agentry fits together", "Agentry",
           "One server keeps the log; you drive it from the browser or Claude Code, and agents work on any machine.")
f.panel(48, 150, 250, 500, "You")
f.panel(330, 150, 520, 500, "Agentry server")
f.panel(882, 150, 270, 500, "Machines")
f.card(68, 190, 210, 84, "The page", ["tasks, Plan, Workspace,", "in any browser"], "blue")
f.card(68, 300, 210, 84, "Claude Code", ["in any repo, with", "agentry mcp"], "blue")
f.card(350, 182, 480, 62, "The page and its API", "everything you see and do", "blue")
f.store(590, 270, 240, 112, "The log", "every event, append-only", ["agentry.db"])
f.card(350, 420, 150, 84, "The gate", ["every action:", "allow, ask, deny"], "amber")
f.card(515, 420, 150, 84, "Watchdog", ["crashes, stalls,", "loops"], "red")
f.card(680, 420, 150, 84, "The loop", ["turns, decisions,", "checks"], "clay")
f.card(350, 548, 480, 64, "Shared files", "kept once, by hash, for every machine", "teal")
f.card(902, 190, 230, 96, "This machine", ["runs agents too, and", "keeps the repos"], "green")
f.card(902, 330, 230, 96, "Workers", ["Linux, macOS, Windows,", "each dialling in"], "green")
f.arrow("M282,212 H346", "blue")
f.arrow("M282,342 H312 V230 H346", "blue")
f.pill(312, 290, "tools", "blue")
f.arrow("M590,248 V266", "teal", both=True, width=1.6)
for x in (470, 590, 700):
    f.arrow(f"M{x},416 V388", "teal", both=True, width=1.6)
f.arrow("M834,462 H866 V384 H898", "clay")
f.pill(866, 426, "turns", "clay")
f.arrow("M1017,430 V526 H425 V508", "amber")
f.pill(700, 526, "every action, through its hook", "amber")
f.arrow("M834,580 H1080 V430", "teal", both=True)
f.pill(960, 580, "files, both ways", "teal")
save(f, "how-it-fits")

# 3. How a task moves --------------------------------------------------------
f = Figure(1200, 700, "How a task moves", "Agentry",
           "The coordinator hands out each step and reads every report; it can't finish until the checks approve the last change.")
hx, hy, hr = 600, 345, 72
f.hub(hx, hy, hr, "Coordinator", ["one per team"])
agents = [(80, 170, "Engineer", "builds", "writes and commits the change"),
          (870, 170, "Reviewer", "checks", "ends with a verdict"),
          (80, 436, "Tester", "checks", "runs it for real, a verdict too"),
          (870, 436, "Deployer", "ships", "merges what was approved")]
for x, y, name, role, sub in agents:
    f.card(x, y, 250, 84, name, sub, "green", kicker=role)
    f.spokes(hx, hy, hr, x + 125, y + 42, 250, 84)
f.rule(568, "A real run", "a fix made after a review goes back for another")
f.chips(640, [("task", "blue"), ("engineer", "green"), ("review: changes", "amber"), ("engineer fixes", "green"),
              ("review: approved", "green"), ("test: approved", "green"), ("deployer merges", "green"), ("done", "clay")])
save(f, "how-a-task-moves")

# 4. What happens to one action ----------------------------------------------
f = Figure(1200, 760, "What happens to one action", "Agentry",
           "Before anything runs, it passes four checks, and every way out that isn't a clear yes blocks it.")
steps = [(150, "The engine calls agentry hook", "before every command, edit or fetch"),
         (262, "It is recorded in the log", "the action, then what was decided"),
         (374, "Built-in protection", "no agent touches the policy, the log or the tokens"),
         (486, "The agent's profile", "its rules, top to bottom: the first match decides")]
for i, (y, title, sub) in enumerate(steps):
    f.number(350, y + 38, i + 1)
    f.card(380, y, 440, 76, title, sub, "amber" if i >= 2 else "muted")
    if i < len(steps) - 1:
        f.arrow(f"M600,{y + 80} V{y + 108}", "muted", width=1.5)
f.card(960, 150, 192, 76, "Denied", "the gate fails closed", "red")
f.arrow("M824,188 H956", "red")
f.pill(890, 188, "server down", "red")
f.card(960, 374, 192, 76, "Denied", "whatever the file says", "red")
f.arrow("M824,412 H956", "red")
f.pill(890, 412, "touches them", "red")
f.arrow("M600,566 V594", "muted", head=False)
f.arrow("M280,594 H920", "muted", head=False)
f.arrow("M280,594 V630", "green")
f.arrow("M600,594 V630", "amber")
f.arrow("M920,594 V630", "red")
f.card(150, 634, 260, 84, "Allow", ["it runs"], "green")
f.card(470, 634, 260, 84, "Ask you", ["it waits: on the page,", "or Claude Code asks you"], "amber")
f.card(790, 634, 260, 84, "Deny", ["refused, with the", "rule's why"], "red")
save(f, "one-action")

# 5. Work across machines ----------------------------------------------------
f = Figure(1200, 680, "Work across machines", "Agentry",
           "A task's branch and its shared files follow the agent to whichever machine works on it next.")
f.panel(48, 150, 300, 300, "A worker: a Linux box")
f.panel(450, 150, 300, 300, "Agentry server")
f.panel(852, 150, 300, 300, "Your Windows PC")
for x, who in ((68, "engineer, turn 3"), (872, "reviewer, turn 4")):
    f.card(x, 190, 260, 64, who, "works in the task's worktree", "green")
    f.card(x, 276, 260, 64, "agentry/task-12", "its branch, committed", "muted", mono=True)
    f.card(x, 362, 260, 64, ".agentry/files", "report.csv, data/", "teal", mono=True)
f.card(470, 190, 260, 84, "The repo", ["the hub every task's", "branch goes through"], "teal")
f.card(470, 300, 260, 84, "Shared files", ["kept once, by hash"], "teal")
f.arrow("M332,308 H392 V232 H466", "muted")
f.pill(392, 270, "hand off", "muted")
f.arrow("M332,394 H420 V342 H466", "teal")
f.pill(420, 368, "read back", "teal")
f.arrow("M734,232 H808 V308 H868", "muted")
f.pill(808, 270, "worktree made", "muted")
f.arrow("M734,342 H780 V394 H868", "teal")
f.pill(780, 368, "delivered", "teal")
f.rule(486, "From anywhere else", "the coordinator can fetch a file or folder from any machine")
f.card(48, 552, 330, 84, "~/Desktop/sales.csv", ["on mac-mini, outside", "its worktrees"], "green", mono=True, kicker="mac-mini")
f.arrow("M382,594 H600 V388", "amber")
f.pill(490, 594, "you approve: Copy it", "amber")
f.card(800, 552, 352, 84, "Inside a machine's worktrees", ["it comes at once; anywhere else,", "only once you allow it"], "amber")
save(f, "across-machines")
