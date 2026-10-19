"""The figure in docs/design, drawn with docs/diagrams' flat kit:
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
