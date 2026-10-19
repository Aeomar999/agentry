"""Wireframes for the three directions in docs/design/activity-readability:
cd docs/design/source && python3 -B activity_directions.py ../activity-readability

Low to mid fidelity, light theme, the page's own tokens. Each sheet shows an
Activity fragment (a long Markdown brief, a bash action, three authors with
you) and a Handoffs fragment, with numbered notes for what a drawing can't show.
"""
import os, sys
from xml.sax.saxutils import escape

OUT = sys.argv[1] if len(sys.argv) > 1 else "."

# The light theme's tokens (ui/src/styles).
S0, S1, S2 = "#ffffff", "#fafafa", "#f3f4f6"
DIV, DIV2 = "#e3e5ea", "#cdd1d8"
INK, INK2, MUTED = "#1f2329", "#424854", "#6b7280"
INFO, OK, WARN = "#0284c7", "#16a34a", "#b45309"
AGENT = {"you": "#64748b", "coordinator": "#2563eb", "engineer": "#0d9488", "reviewer": "#7c3aed",
         "tech-lead": "#ea580c", "product-lead": "#b45309", "product-designer": "#db2777"}
# Monaco's light colours for shell.
KW, STR, OP = "#0000ff", "#a31515", "#6b7280"
SANS = "Inter, ui-sans-serif, system-ui, sans-serif"
MONO = "'JetBrains Mono', ui-monospace, monospace"


class Sheet:
    def __init__(self, w, h, title, sub):
        self.w, self.h, self.out = w, h, []
        self.rect(0, 0, w, h, "#f6f7f9")
        self.text(40, 46, title, 22, INK, 700)
        self.text(40, 72, sub, 13, MUTED)

    def rect(self, x, y, w, h, fill="none", stroke=None, r=0, opacity=None, dash=None):
        a = f' stroke="{stroke}" stroke-width="1"' if stroke else ""
        a += f' fill-opacity="{opacity}"' if opacity is not None else ""
        a += f' stroke-dasharray="{dash}"' if dash else ""
        self.out.append(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}" fill="{fill}"{a}/>')

    def line(self, x1, y1, x2, y2, color=DIV, dash=None):
        d = f' stroke-dasharray="{dash}"' if dash else ""
        self.out.append(f'<line x1="{x1}" y1="{y1}" x2="{x2}" y2="{y2}" stroke="{color}" stroke-width="1"{d}/>')

    def text(self, x, y, s, size=12, fill=INK, weight=400, mono=False, anchor="start", underline=False):
        u = ' text-decoration="underline"' if underline else ""
        self.out.append(
            f'<text x="{x}" y="{y}" font-family="{MONO if mono else SANS}" font-size="{size}" '
            f'font-weight="{weight}" fill="{fill}" text-anchor="{anchor}"{u}>{escape(s)}</text>')

    def spans(self, x, y, parts, size=12.5, mono=False):
        """One line of differently styled runs: (text, fill, weight, mono, underline)."""
        out = f'<text x="{x}" y="{y}" font-size="{size}" font-family="{MONO if mono else SANS}">'
        for p in parts:
            s, fill, weight, m, u = (list(p) + [INK2, 400, mono, False][len(p) - 1:])[:5]
            fam = MONO if m else SANS
            dec = ' text-decoration="underline"' if u else ""
            out += f'<tspan fill="{fill}" font-weight="{weight}" font-family="{fam}"{dec}>{escape(s)}</tspan>'
        self.out.append(out + "</text>")

    def chip(self, x, y, who, size=20):
        """The Avatar: the first letter on a tint of the agent's colour."""
        c = AGENT[who]
        self.rect(x, y, size, size, c, r=3, opacity=0.16)
        self.text(x + size / 2, y + size * 0.7, "Y" if who == "you" else who[0].upper(), size * 0.55, c, 700, True, "middle")

    def badge(self, x, y, label, color=MUTED):
        w = 8 + len(label) * 6.2
        self.rect(x, y, w, 18, S0, DIV2, 3)
        self.text(x + w / 2, y + 13, label, 11, color, 500, anchor="middle")
        return w

    def button(self, x, y, label):
        w = 14 + len(label) * 6.4
        self.rect(x, y, w, 24, S0, DIV2, 3)
        self.text(x + 7, y + 16, label, 11.5, INK2, 500)
        return w

    def arrow(self, x, y, w=12, color=MUTED):
        """A right arrow from (x, y) on the text's middle line."""
        self.out.append(f'<path d="M{x},{y} H{x + w} M{x + w - 4},{y - 4} L{x + w},{y} L{x + w - 4},{y + 4}" '
                        f'fill="none" stroke="{color}" stroke-width="1.4"/>')

    def note(self, x, y, n):
        self.rect(x, y, 18, 18, INK, r=2)
        self.text(x + 9, y + 13, str(n), 11, S0, 700, anchor="middle")

    def code(self, x, y, w, lines, label=None):
        """A highlighted code box: lines are lists of (text, colour) runs."""
        h = 12 + 18 * len(lines)
        self.rect(x, y, w, h, S1, DIV, 3)
        for i, runs in enumerate(lines):
            self.spans(x + 10, y + 21 + 18 * i, [(t, c, 400, True) for t, c in runs], 12, True)
        if label:
            self.text(x + w - 8, y + 15, label, 10.5, MUTED, 500, True, "end")
        return h

    def notes(self, x, y, w, items):
        self.text(x, y, "Notes", 13, INK, 700)
        y += 22
        for n, body in items:
            self.note(x, y - 13, n)
            for i, part in enumerate(body):
                self.text(x + 28, y + 17 * i, part, 12, INK2)
            y += 17 * len(body) + 10

    def panel(self, x, y, w, h, title):
        self.text(x, y - 10, title, 12, MUTED, 600)
        self.rect(x, y, w, h, S0, DIV2, 2)

    def save(self, name):
        svg = (f'<svg xmlns="http://www.w3.org/2000/svg" width="{self.w}" height="{self.h}" '
               f'viewBox="0 0 {self.w} {self.h}" role="img">' + "".join(self.out) + "</svg>\n")
        with open(os.path.join(OUT, name), "w") as f:
            f.write(svg)


BASH_LONG = [
    [("cd", KW), (" ui ", INK), ("&&", OP), (" npm", KW), (" run lint ", INK), ("&&", OP), (" npm", KW), (" run build ", INK), ("&&", OP)],
    [("  cd", KW), (" .. ", INK), ("&&", OP), (" go", KW), (" test ./internal/web/... ", INK), ("-run", INK), (" ", INK), ("'TestDist|TestTask'", STR)],
    [("  ", INK), ("&&", OP), (" git", KW), (" add internal/web/dist ui/src ", INK), ("&&", OP), (" git", KW), (" commit ", INK), ("-m", INK)],
]
BASH_SHORT = [[("git", KW), (" diff dev...HEAD ", INK), ("--stat", INK)]]
BRIEF_FENCE = [[("cd", KW), (" ui ", INK), ("&&", OP), (" npm", KW), (" run lint ", INK), ("&&", OP), (" npm", KW), (" run build", INK)]]


def toolbar(s, x, y, w, extra=None, count=True):
    s.rect(x, y, w, 44, S0)
    s.line(x, y + 44, x + w, y + 44)
    s.rect(x + 24, y + 9, 228, 26, S2, DIV, 3)
    s.rect(x + 26, y + 11, 84, 22, S0, DIV2, 2)
    s.text(x + 68, y + 26, "Everything", 11.5, INK, 600, anchor="middle")
    s.text(x + 140, y + 26, "Actions", 11.5, MUTED, 500, anchor="middle")
    s.text(x + 206, y + 26, "Messages", 11.5, MUTED, 500, anchor="middle")
    if extra:
        extra(x, y)
    if count:
        s.text(x + w - 24, y + 27, "214 rows", 11, MUTED, 400, True, "end")


# ---------------------------------------------------------------- Direction A
def direction_a():
    s = Sheet(1280, 1060, "Direction A · Author bands",
              "The feed is cut into sections, one per author: every turn's band names its agent and stays on top while its rows scroll under it.")
    X, Y, W, H = 40, 120, 780, 780
    s.panel(X, Y, W, H, "ACTIVITY  ·  1280 wide")
    toolbar(s, X, Y, W)
    R = X + W - 120  # the time rail
    s.line(R + 14, Y + 44, R + 14, Y + H)

    def band(y, who, turn, team, role, status, tok, time, dur, sticky=False):
        s.rect(X + 1, y, W - 2, 44, S1)
        s.line(X, y, X + W, y)
        s.line(X, y + 44, X + W, y + 44)
        s.chip(X + 24, y + 12, who)
        s.spans(X + 54, y + 27, [(who, INK, 700, True), (f"   {team}" + (f" · {role}" if role else ""), MUTED, 400)], 12.5)
        bx = X + 54 + len(who) * 7.6 + len(team + role) * 6.4 + 50
        if turn:
            s.text(bx, y + 27, turn, 11, MUTED, 400, True)
            bx += len(turn) * 7 + 10
        if status:
            bx += s.badge(bx, y + 13, status, INFO if status == "running" else MUTED) + 8
        if tok:
            s.text(bx, y + 27, tok, 11, MUTED, 400, True)
        s.rect(R + 10, y + 18, 9, 9, AGENT[who] if who != "you" else MUTED, r=4.5)
        s.text(R + 28, y + 22, time, 11, MUTED, 400, True)
        if dur:
            s.text(R + 28, y + 36, dur, 11, OK if dur.endswith("…") else MUTED, 400, True)

    def rail(y, time):
        s.rect(R + 10, y + 4, 9, 9, MUTED, r=4.5)
        s.text(R + 28, y + 13, time, 11, MUTED, 400, True)

    y = Y + 44
    band(y, "reviewer", "turn-5", "Software", "reviews", "running", "12k tokens", "06:12", "4m…")
    s.note(X + W - 160, y + 13, 1)
    y += 52
    s.text(X + 54, y + 13, "Bash", 11, MUTED, 400, True)
    s.code(X + 54, y + 20, 500, BASH_SHORT)
    s.badge(X + 572, y + 22, "running", INFO)
    rail(y + 8, "06:13")
    y += 72

    band(y, "engineer", "turn-4", "Software", "", "ended", "41k tokens", "05:21", "48m")
    y += 56
    s.text(X + 54, y + 4, "Finished, 41k tokens", 12, OK, 600)
    rail(y - 8, "06:09")
    s.spans(X + 54, y + 26, [("Done.", INK, 700), (" Lint, build and ", INK2), ("go test ./...", INK, 400, True), (" pass. Committed ", INK2),
                             ("3f2a9c1", INFO, 400, True, True), (".", INK2)])
    s.spans(X + 62, y + 48, [("•  Author bands in ", INK2), ("ActivityFeed.tsx", INFO, 400, True, True), (" and the turn rows", INK2)])
    s.spans(X + 62, y + 68, [("•  Links: ", INK2), ("task-21", INFO, 400, True, True), (" opens the task, files open the Workspace", INK2)])
    s.note(X + 640, y + 14, 4)
    y += 90
    s.text(X + 54, y + 4, "Bash", 11, MUTED, 400, True)
    s.code(X + 54, y + 12, 560, BASH_LONG)
    s.badge(X + 630, y + 14, "done")
    s.text(X + 54, y + 92, "Show all · 9 lines", 11.5, INK2, 600, underline=True)
    s.note(X + 182, y + 79, 3)
    rail(y, "05:58")
    y += 112

    band(y, "coordinator", "turn-3", "Software", "coordinator", "ended", "8k tokens", "05:12", "9m")
    y += 56
    s.chip(X + 54, y - 11, "coordinator", 18)
    s.text(X + 78, y + 4, "coordinator", 12.5, INK, 700, True)
    s.arrow(X + 170, y)
    s.chip(X + 190, y - 11, "engineer", 18)
    s.text(X + 214, y + 4, "engineer", 12.5, INK, 700, True)
    s.text(X + 290, y + 4, "turn-4", 12, INFO, 400, True, True)
    rail(y - 8, "05:20")
    s.text(X + 54, y + 32, "Build the author bands", 14, INK, 700)
    s.spans(X + 54, y + 56, [("Read ", INK2), ("spec.md", INFO, 400, True, True), (" and ", INK2),
                             ("the directions", INFO, 400, False, True), (" first. Then:", INK2)])
    s.spans(X + 62, y + 78, [("1.  Render briefs and reports with ", INK2), ("Markdown", INK, 400, True), (" in compact mode", INK2)])
    s.spans(X + 62, y + 98, [("2.  Highlight commands lazily, as ", INK2), ("MonacoColorize", INK, 400, True), (" does", INK2)])
    s.code(X + 54, y + 110, 560, BRIEF_FENCE, "bash")
    s.line(X + 54, y + 152, X + 614, y + 152, DIV)
    s.text(X + 54, y + 172, "Show all · 38 lines", 11.5, INK2, 600, underline=True)
    s.note(X + 640, y + 18, 2)
    s.note(X + 192, y + 159, 5)
    y += 192

    band(y, "you", "", "", "", "", "", "05:11", "")
    y += 56
    s.chip(X + 54, y - 11, "you", 18)
    s.text(X + 78, y + 4, "you", 12.5, INK, 700, True)
    s.arrow(X + 108, y)
    s.chip(X + 128, y - 11, "coordinator", 18)
    s.text(X + 152, y + 4, "coordinator", 12.5, INK, 700, True)
    rail(y - 8, "05:11")
    s.spans(X + 54, y + 26, [("Make Activity and Handoffs readable, like ", INK2), ("task-21", INFO, 400, True, True),
                             (" did for the chat. See ", INK2), ("github.com/…/issues/88", INFO, 400, False, True), (" ↗", INFO)])
    s.note(X + 640, y + 12, 6)

    # Handoffs
    HX, HW = 860, 380
    s.panel(HX, Y, HW, 520, "HANDOFFS")
    s.line(HX + 22, Y + 30, HX + 22, Y + 500)

    def card(y, frm, to, time, title, lines, turn, h, long=True):
        s.rect(HX + 17, y + 12, 11, 11, AGENT[frm], r=5.5)
        s.rect(HX + 40, y, HW - 56, h, S0, DIV, 3)
        s.chip(HX + 52, y + 10, frm, 18)
        s.text(HX + 76, y + 24, frm, 12, INK, 700, True)
        ax = HX + 80 + len(frm) * 7.4
        s.arrow(ax - 2, y + 20, 10)
        s.chip(ax + 16, y + 10, to, 18)
        s.text(ax + 40, y + 24, to, 12, INK, 700, True)
        s.text(HX + HW - 26, y + 24, time, 11, MUTED, 400, True, "end")
        s.text(HX + 52, y + 50, title, 13, INK, 700)
        for i, l in enumerate(lines):
            s.spans(HX + 52, y + 72 + 19 * i, l, 12)
        if long:
            s.text(HX + 52, y + h - 40, "Show all · 38 lines", 11, INK2, 600, underline=True)
        s.line(HX + 40, y + h - 28, HX + HW - 16, y + h - 28)
        s.spans(HX + 52, y + h - 10, [("Led to  ", MUTED), (turn, INK2, 400, True), ("  ·  48m  ·  14 actions", MUTED)], 11)
        s.text(HX + HW - 26, y + h - 10, "Activity ›", 11, MUTED, 500, anchor="end")

    card(Y + 20, "coordinator", "engineer", "05:20", "Build the author bands",
         [[("Read ", INK2), ("spec.md", INFO, 400, True, True), (" and ", INK2), ("the directions", INFO, 400, False, True), (" first.", INK2)],
          [("1.  Render briefs with ", INK2), ("Markdown", INK, 400, True)],
          [("2.  Highlight commands lazily", INK2)]], "turn-4 engineer", 180)
    card(Y + 216, "tech-lead", "engineer", "04:02", "Split the work in two",
         [[("Links first, then the bands. Keep ", INK2), ("Clamp", INK, 400, True)], [("for actions.", INK2)]], "turn-2 engineer", 140, False)
    card(Y + 372, "you", "coordinator", "03:40", "Make Activity readable",
         [[("Like ", INK2), ("task-21", INFO, 400, True, True), (" did for the chat.", INK2)]], "turn-1 coordinator", 120, False)
    s.note(HX + HW - 34, Y + 60, 7)

    s.notes(860, 680, 380, [
        (1, ["The band sticks under the toolbar while its", "rows scroll; it carries chip, name, team, role.", "Every row in a turn is that agent's work."]),
        (2, ["A brief: compact Markdown, cut at ~10 lines", "on a clean line, no fade. ‘Show all · 38 lines’", "is a button with aria-expanded."]),
        (3, ["Commands wrap at spaces, 3 lines, then", "‘Show all’. Monaco colours them only once", "they scroll into view; plain mono until then."]),
        (4, ["Links are underlined, not colour alone. A SHA", "of the branch opens it in the Workspace."]),
        (5, ["Fenced code: same box, language top right."]),
        (6, ["Rows outside a turn get a band too: ‘you’.", "External links open a new tab, with ↗."]),
        (7, ["Handoffs: the brief’s first heading is the card", "title; the whole card stays a link to the turn."]),
    ])
    s.text(40, 940, "At 390 wide: the rail becomes the time alone (56px) and the band drops the team, keeping chip, name and status;", 12, INK2)
    s.text(40, 960, "code boxes scroll sideways only for words longer than the line. Dark theme: same tokens, chips tint at 22%.", 12, INK2)
    s.save("direction-a.svg")


# ---------------------------------------------------------------- Direction B
def direction_b():
    s = Sheet(1280, 1030, "Direction B · Conversation",
              "Activity reads like the coordinator chat: an avatar and byline per run of one author, turns as quiet dividers, filters by agent.")
    X, Y, W, H = 40, 120, 780, 810
    s.panel(X, Y, W, H, "ACTIVITY  ·  1280 wide")

    def filters(x, y):
        cx = x + 272
        for who, on in (("All", True), ("you", False), ("coordinator", False), ("engineer", False), ("reviewer", False)):
            w = 18 + len(who) * 6.6 + (22 if who != "All" else 0)
            s.rect(cx, y + 9, w, 26, S2 if on else S0, DIV2, 3)
            tx = cx + 9
            if who != "All":
                s.rect(cx + 8, y + 17, 10, 10, AGENT[who], r=5)
                tx += 16
            s.text(tx, y + 26, who, 11.5, INK if on else INK2, 600 if on else 500, who != "All")
            cx += w + 6
    toolbar(s, X, Y, W, filters, count=False)
    s.note(X + 740, Y + 13, 1)

    def divider(y, label):
        s.line(X + 24, y, X + 250, y)
        s.text(X + W / 2, y + 4, label, 11, MUTED, 500, True, "middle")
        s.line(X + W - 250, y, X + W - 24, y)

    def byline(y, who, team, time, to=None, sticky=False):
        if sticky:
            s.rect(X + 1, y - 20, W - 2, 36, S0)
            s.line(X, y + 16, X + W, y + 16)
        s.chip(X + 24, y - 15, who, 28)
        s.spans(X + 64, y + 4, [(who, INK, 700, True), (f"   {team}", MUTED), (f"   {time}", MUTED, 400, True)], 12.5)
        if to:
            tx = X + 64 + len(who + team + time) * 7.2 + 40
            s.arrow(tx, y)
            s.chip(tx + 20, y - 9, to, 18)
            s.text(tx + 44, y + 4, to, 12.5, INK, 700, True)

    y = Y + 76
    byline(y, "reviewer", "Software · reviews", "06:13", sticky=True)
    s.note(X + W - 40, y - 10, 2)
    s.text(X + 64, y + 30, "Bash", 11, MUTED, 400, True)
    s.code(X + 64, y + 38, 520, BASH_SHORT)
    s.badge(X + 600, y + 40, "running", INFO)
    y += 96
    divider(y, "turn-5 · reviewer started · 06:12")
    y += 40

    byline(y, "engineer", "Software", "06:09")
    s.text(X + 64, y + 30, "Finished, 41k tokens", 12, OK, 600)
    s.spans(X + 64, y + 52, [("Done.", INK, 700), (" Lint, build and ", INK2), ("go test ./...", INK, 400, True), (" pass. Committed ", INK2),
                             ("3f2a9c1", INFO, 400, True, True), (".", INK2)])
    s.spans(X + 72, y + 74, [("•  Links: ", INK2), ("task-21", INFO, 400, True, True), (" opens the task, files open the Workspace", INK2)])
    s.text(X + 64, y + 104, "Bash", 11, MUTED, 400, True)
    s.code(X + 64, y + 112, 560, BASH_LONG)
    s.badge(X + 640, y + 114, "done")
    s.text(X + 64, y + 190, "Show all · 9 lines", 11.5, INK2, 600, underline=True)
    s.note(X + 190, y + 177, 4)
    s.text(X + 64, y + 216, "Read its brief  ·  Updated its plan: 3 of 5 steps done", 12, MUTED)
    s.note(X + 420, y + 203, 3)
    y += 244
    divider(y, "turn-4 · engineer started · 05:21")
    y += 40

    byline(y, "coordinator", "Software · coordinator", "05:20", to="engineer")
    s.text(X + 64, y + 32, "Build the author bands", 14, INK, 700)
    s.spans(X + 64, y + 56, [("Read ", INK2), ("spec.md", INFO, 400, True, True), (" and ", INK2),
                             ("the directions", INFO, 400, False, True), (" first. Then:", INK2)])
    s.spans(X + 72, y + 78, [("1.  Render briefs and reports with ", INK2), ("Markdown", INK, 400, True), (" in compact mode", INK2)])
    s.spans(X + 72, y + 98, [("2.  Highlight commands lazily, as ", INK2), ("MonacoColorize", INK, 400, True), (" does", INK2)])
    s.code(X + 64, y + 110, 560, BRIEF_FENCE, "bash")
    s.spans(X + 72, y + 168, [("3.  Group rows by author; keep the byline on top of a long entry", INK2)])
    s.text(X + 64, y + 196, "Show all · 38 lines", 11.5, INK2, 600, underline=True)
    s.note(X + 650, y + 20, 5)
    y += 228
    divider(y, "turn-3 · coordinator started · 05:12")
    y += 40

    byline(y, "you", "", "05:11", to="coordinator")
    s.spans(X + 64, y + 28, [("Make Activity and Handoffs readable, like ", INK2), ("task-21", INFO, 400, True, True),
                             (" did for the chat. See ", INK2), ("github.com/…/issues/88", INFO, 400, False, True), (" ↗", INFO)])

    # Handoffs
    HX, HW = 860, 380
    s.panel(HX, Y, HW, 460, "HANDOFFS")

    def entry(y, frm, to, time, title, lines, turn, long=False):
        s.chip(HX + 16, y, frm, 28)
        s.text(HX + 54, y + 18, frm, 12, INK, 700, True)
        s.arrow(HX + 60 + len(frm) * 7.4, y + 14, 10)
        s.text(HX + 78 + len(frm) * 7.4, y + 18, to, 12, INK, 700, True)
        s.text(HX + HW - 16, y + 18, time, 11, MUTED, 400, True, "end")
        s.text(HX + 54, y + 42, title, 13, INK, 700)
        for i, l in enumerate(lines):
            s.spans(HX + 54, y + 64 + 19 * i, l, 12)
        ly = y + 64 + 19 * len(lines) + 4
        more = [("Show all", INK2, 600, False, True), ("   ·   ", MUTED)] if long else []
        s.spans(HX + 54, ly, more + [(turn, INFO, 400, True, True), ("  48m · 14 actions", MUTED)], 11)
        return ly + 30

    y = entry(Y + 20, "coordinator", "engineer", "05:20", "Build the author bands",
              [[("Read ", INK2), ("spec.md", INFO, 400, True, True), (" and ", INK2), ("the directions", INFO, 400, False, True), (" first.", INK2)],
               [("1.  Render briefs with ", INK2), ("Markdown", INK, 400, True)]], "turn-4", True)
    y = entry(y, "tech-lead", "engineer", "04:02", "Split the work in two",
              [[("Links first, then the bands. Keep ", INK2), ("Clamp", INK, 400, True), (".", INK2)]], "turn-2")
    y = entry(y, "you", "coordinator", "03:40", "Make Activity readable",
              [[("Like ", INK2), ("task-21", INFO, 400, True, True), (" did for the chat.", INK2)]], "turn-1")
    s.note(HX + HW - 34, Y + 60, 6)

    s.notes(860, 620, 380, [
        (1, ["Filter chips by agent: name in text and a dot.", "Several can be on; ‘All’ clears them."]),
        (2, ["A long entry’s byline sticks while you read it,", "then gives way to the next one."]),
        (3, ["Notes fold into one quiet line under the", "author, as the chat folds ‘updates’."]),
        (4, ["Commands as in A: wrap, 3 lines, lazy colour."]),
        (5, ["A handoff names its addressee in the byline:", "‘coordinator to engineer’, both as text."]),
        (6, ["Handoffs read the same as Activity: one look", "for both tabs and the chat."]),
    ])
    s.text(40, 970, "No time rail: time moves into the byline, and running/durations into the turn dividers. At 390 wide the avatar", 12, INK2)
    s.text(40, 990, "shrinks to 20px and the filter chips scroll sideways. Roomiest of the three: about 30% fewer rows on screen than today.", 12, INK2)
    s.save("direction-b.svg")


# ---------------------------------------------------------------- Direction C
def direction_c():
    s = Sheet(1280, 910, "Direction C · Ledger",
              "Dense rows with an author column on every row, so attribution never depends on scrolling; a Compact / Detailed switch and an agent filter.")
    X, Y, W, H = 40, 120, 780, 680
    s.panel(X, Y, W, H, "ACTIVITY  ·  1280 wide  ·  Compact")

    def extra(x, y):
        s.rect(x + 268, y + 9, 128, 26, S0, DIV2, 3)
        s.text(x + 278, y + 26, "Agent: all  ▾", 11.5, INK2, 500)
        s.rect(x + 406, y + 9, 164, 26, S2, DIV, 3)
        s.rect(x + 408, y + 11, 78, 22, S0, DIV2, 2)
        s.text(x + 447, y + 26, "Compact", 11.5, INK, 600, anchor="middle")
        s.text(x + 528, y + 26, "Detailed", 11.5, MUTED, 500, anchor="middle")
    toolbar(s, X, Y, W, extra)
    s.note(X + 580, Y + 13, 1)
    A, C, T = X + 24, X + 196, X + W - 70  # author, content, time columns
    s.line(C - 14, Y + 44, C - 14, Y + H, DIV)

    def row(y, who, h, time, kind=None):
        s.line(X, y + h, X + W, y + h, DIV)
        s.chip(A, y + 8, who, 18)
        s.text(A + 26, y + 21, who, 12, INK, 600, True)
        s.text(T, y + 21, time, 11, MUTED, 400, True)

    def turn(y, label):
        s.rect(X + 1, y, W - 2, 28, S1)
        s.line(X, y + 28, X + W, y + 28)
        s.text(C, y + 19, label, 11, MUTED, 500, True)
        return y + 28

    y = Y + 44
    y = turn(y, "turn-5 · reviewer · running · 4m…")
    row(y, "reviewer", 36, "06:13")
    s.text(C, y + 21, "Bash", 11, MUTED, 400, True)
    s.spans(C + 40, y + 21, [("git", KW), (" diff dev...HEAD ", INK), ("--stat", INK)], 12, True)
    s.badge(T - 70, y + 9, "running", INFO)
    y += 36
    row(y, "reviewer", 32, "06:12")
    s.text(C, y + 21, "Read its brief", 12, MUTED)
    y += 32
    y = turn(y, "turn-4 · engineer · ended · 48m · 41k tokens")
    row(y, "engineer", 36, "06:09")
    s.spans(C, y + 21, [("Finished", OK, 600), ("  Done. Lint, build and ", INK2), ("go test ./...", INK, 400, True), (" pass. Committed ", INK2),
                        ("3f2a9c1", INFO, 400, True, True)], 12)
    s.text(T - 12, y + 21, "▸", 12, MUTED, anchor="end")
    s.note(C - 40, y + 9, 2)
    y += 36
    row(y, "engineer", 36, "05:58")
    s.text(C, y + 21, "Bash", 11, MUTED, 400, True)
    s.spans(C + 40, y + 21, [("cd", KW), (" ui ", INK), ("&&", OP), (" npm", KW), (" run lint ", INK), ("&&", OP), (" npm", KW), (" run build ", INK), ("&& …", OP)], 12, True)
    s.text(T - 12, y + 21, "9 lines ▸", 11, MUTED, anchor="end")
    s.note(C - 40, y + 9, 3)
    y += 36
    for t, label in (("05:40", "Updated its plan: 3 of 5 steps done"), ("05:22", "Read its brief")):
        row(y, "engineer", 32, t)
        s.text(C, y + 21, label, 12, MUTED)
        y += 32
    y = turn(y, "turn-3 · coordinator · ended · 9m · 8k tokens")
    # one row opened, as Detailed shows them all
    h = 230
    row(y, "coordinator", h, "05:20")
    s.arrow(C, y + 17)
    s.spans(C + 18, y + 21, [("engineer", INK, 700, True), ("   Build the author bands   ", INK, 700), ("turn-4", INFO, 400, True, True)], 12.5)
    s.text(T - 12, y + 21, "▾", 12, MUTED, anchor="end")
    s.spans(C, y + 48, [("Read ", INK2), ("spec.md", INFO, 400, True, True), (" and ", INK2), ("the directions", INFO, 400, False, True), (" first. Then:", INK2)])
    s.spans(C + 8, y + 70, [("1.  Render briefs and reports with ", INK2), ("Markdown", INK, 400, True), (" in compact mode", INK2)])
    s.spans(C + 8, y + 90, [("2.  Highlight commands lazily", INK2)])
    s.code(C, y + 102, 470, BRIEF_FENCE, "bash")
    s.spans(C + 8, y + 160, [("3.  Group rows by author", INK2)])
    s.text(C, y + 190, "Show all · 38 lines", 11.5, INK2, 600, underline=True)
    s.text(A, y + 50, "Software", 11, MUTED)
    s.text(A, y + 66, "coordinator", 11, MUTED)
    s.note(C - 40, y + 40, 4)
    y += h
    row(y, "coordinator", 32, "05:13")
    s.text(C, y + 21, "Read its brief", 12, MUTED)
    y += 32
    row(y, "you", 36, "05:11")
    s.arrow(C, y + 17)
    s.spans(C + 18, y + 21, [("coordinator", INK, 700, True), ("   Make Activity and Handoffs readable, like ", INK2),
                        ("task-21", INFO, 400, True, True), (" …", INK2)], 12.5)
    s.text(T - 12, y + 21, "▸", 12, MUTED, anchor="end")
    y += 36
    row(y, "you", 32, "05:10")
    s.text(C, y + 21, "Budget set to unlimited", 12, MUTED)
    s.note(X + 160, Y + 60, 5)

    # Handoffs
    HX, HW = 860, 380
    s.panel(HX, Y, HW, 400, "HANDOFFS")
    s.rect(HX + 1, Y + 1, HW - 2, 30, S1)
    s.line(HX, Y + 31, HX + HW, Y + 31)
    for x, label in ((HX + 16, "From, to"), (HX + 150, "Brief"), (HX + HW - 16, "When")):
        s.text(x, Y + 20, label, 11, MUTED, 600, anchor="end" if label == "When" else "start")
    hy = Y + 31
    for frm, to, title, time, opened in (("coordinator", "engineer", "Build the author bands", "05:20", True),
                                         ("tech-lead", "engineer", "Split the work in two", "04:02", False),
                                         ("engineer", "tech-lead", "Done: links and bands", "03:58", False),
                                         ("you", "coordinator", "Make Activity readable", "03:40", False)):
        h = 160 if opened else 52
        s.chip(HX + 16, hy + 10, frm, 16)
        s.text(HX + 38, hy + 22, frm, 11.5, INK, 600, True)
        s.arrow(HX + 38, hy + 36, 9)
        s.text(HX + 52, hy + 40, to, 11.5, INK2, 500, True)
        s.text(HX + 150, hy + 22, title, 12, INK, 600)
        s.text(HX + HW - 16, hy + 22, time, 11, MUTED, 400, True, "end")
        if opened:
            s.spans(HX + 150, hy + 44, [("Read ", INK2), ("spec.md", INFO, 400, True, True), (" first.", INK2)], 12)
            s.spans(HX + 150, hy + 64, [("1.  Markdown, compact", INK2)], 12)
            s.spans(HX + 150, hy + 84, [("2.  Lazy highlighting", INK2)], 12)
            s.text(HX + 150, hy + 108, "Show all · 38 lines", 11, INK2, 600, underline=True)
            s.spans(HX + 150, hy + 138, [("turn-4", INFO, 400, True, True), ("  48m · 14 actions", MUTED)], 11)
            s.text(HX + HW - 16, hy + 138, "Activity ›", 11, MUTED, 500, anchor="end")
        s.line(HX, hy + h, HX + HW, hy + h)
        hy += h
    s.note(HX + 210, Y + 6, 6)

    s.notes(860, 560, 380, [
        (1, ["Agent filter (a menu, for ten agents and", "more) and the Compact / Detailed switch."]),
        (2, ["Compact: one line per row; ▸ opens the row", "in place. Detailed opens every row."]),
        (3, ["A command shows its first line; opened, it", "wraps and is coloured, as in A."]),
        (4, ["Opened rows add team and role under the", "author, and render the whole Markdown."]),
        (5, ["Chip and name on every row: nothing to", "scroll for, at the cost of repetition."]),
        (6, ["Handoffs as a table; one row opens at a time."]),
    ])
    s.text(40, 850, "At 390 wide the author column folds into a line above each row (chip and name), so rows grow to two lines.", 12, INK2)
    s.text(40, 870, "Densest of the three: about twice today’s rows on screen in Compact.", 12, INK2)
    s.save("direction-c.svg")


direction_a()
direction_b()
direction_c()
