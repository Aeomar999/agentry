"""The figures in docs/design, drawn with docs/diagrams' flat kit:
cd docs/design/source && python3 -B figures.py .."""
import os, sys
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "diagrams", "source"))
from flat import Figure

OUT = sys.argv[1]

f = Figure(1200, 640, "The read model", "Agentry",
           "Every write goes through Propose; readers see an event only once its state is applied in memory.")
f.panel(48, 150, 760, 250, "Store.Propose, one at a time")
steps = [(68, "1  Check", ["rules.Check reads", "current.State"], "amber"),
         (252, "2  Append", ["INSERT in the", "transaction"], "teal"),
         (436, "3  Commit", ["the event is", "in the log"], "teal"),
         (620, "4  Apply", ["its stored form", "to current.State"], "green")]
for x, title, sub, color in steps:
    f.card(x, 190, 168, 84, title, sub, color)
for x in (236, 420, 604):
    f.arrow(f"M{x+2},232 H{x+14}", "muted", width=1.5)
f.card(68, 300, 352, 76, "Refused or rolled back", ["a refusal stores proposal.rejected, which no", "state reads; a failed commit applies nothing"], "red")
f.card(436, 300, 352, 76, "5  Mark applied, notify", ["applied = its position; reads of the log", "stop there, so none sees it before its state"], "blue")
f.arrow("M704,278 V296", "muted", width=1.5)
f.store(980, 170, 240, 116, "The log", "agentry.db, append-only", ["the source of truth"])
f.arrow("M520,186 V164 H980 V166", "teal")
f.pill(760, 164, "commit", "teal")
f.card(860, 318, 240, 76, "At startup", ["replays the log into current.State:", "about 0.2 s, about 1.3 MB held"], "teal")
f.arrow("M980,290 V314", "teal")
f.arrow("M860,356 H812", "teal")
f.rule(432, "Readers", "take a snapshot under a read lock: a task they hold is never changed again")
readers = [(48, "The page's API", ["tasks, questions, agents,", "one task: no SQL, no replay"], "blue"),
           (334, "Coordinator loop", ["the task it acts on", "and the roster"], "clay"),
           (620, "Watchdog and hooks", ["every open turn each second;", "the task of each tool call"], "red"),
           (906, "The log, when asked", ["from SQLite, never past the", "applied position (step 5)"], "teal")]
for x, title, sub, color in readers:
    f.card(x, 484, 246, 84, title, sub, color)
f.note(48, 604, "Readers never write to the state; only step 4 does, under the store's lock.", size=12.5)
with open(f"{OUT}/read-model.svg", "w") as out:
    out.write(f.svg())

f = Figure(1200, 800, "How a feature plugs in", "Agentry",
           "A feature registers once, in serve.go. Each part lands in one core slot, and the core never names it.")
f.panel(250, 140, 440, 620, "Core: names no feature")
f.panel(790, 140, 362, 620, "The Ideas feature, internal/ideas")
rows = [("events catalogue", "types, writers, a stream prefix", "teal",
         "Types in idea-N streams", "idea.suggested, accepted, dismissed"),
        ("Store.Propose, rules.Check", "check 3 under the one write lock", "amber",
         "Streams.Check", "suggested once, decided once while open"),
        ("current.State", "one applier, rebuilt from the log", "teal",
         "FromEvents, Next", "the ideas slice, built and applied"),
        ("Coordinator loop", "briefs and done decisions", "clay",
         "Coordinator hook", "on done, a Visionaries lead's ideas"),
        ("web: routes behind requireUI", "every write made as you", "blue",
         "Routes", "/api/ideas, accept, dismiss"),
        ("GET /api/features", "the manifest the page follows", "blue",
         "Manifest", "on while the team or any idea exists"),
        ("mcp: tools and waiting", "for Claude Code sessions", "blue",
         "Tools, Waiting", "list_ideas, accept_idea, dismiss_idea"),
        ("UI shell: router, sidebar, Ctrl K", "shows what the manifest says", "green",
         "FeatureUI, ui/src/features/ideas", "the /ideas page, nav item, command")]
for i, (core, core_sub, color, mine, mine_sub) in enumerate(rows):
    y = 184 + 70 * i
    f.card(270, y, 400, 56, core, core_sub, color)
    f.card(808, y, 326, 56, mine, mine_sub, color)
    f.arrow(f"M802,{y+28} H676", color, width=1.5)
f.store(130, 250, 160, 120, "The log", "agentry.db")
f.note(130, 400, "Propose commits;", anchor="middle")
f.note(130, 418, "State replays it", anchor="middle")
f.arrow("M266,282 H214", "teal", width=1.5)
f.arrow("M214,352 H266", "teal", width=1.5)
f.note(48, 784, "Without the feature's code, its events stay in the log, are skipped and counted on replay, and new ones are refused.", size=12.5)
with open(f"{OUT}/extensions.svg", "w") as out:
    out.write(f.svg())
