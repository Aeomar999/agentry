"""Wireframes for Direction A, Author bands, in docs/design/activity-readability:
cd docs/design/source && python3 -B activity_bands.py ../activity-readability

Mid fidelity, the page's own tokens, light and dark. Writes activity-desktop,
activity-mobile, handoffs, states and activity-dark. Text is measured with
Noto Sans's widths (what the headless renderer falls back to for Inter) and
0.6em for mono, so links, icons and badges land after the words they follow.
"""
import os, sys
from xml.sax.saxutils import escape

OUT = sys.argv[1] if len(sys.argv) > 1 else "."
SANS = "Inter, 'Noto Sans', ui-sans-serif, system-ui, sans-serif"
MONO = "'JetBrains Mono', 'DejaVu Sans Mono', ui-monospace, monospace"

# Noto Sans advance widths per 1000 units, ASCII 32..126, regular and bold.
_REG = [260, 269, 408, 646, 572, 831, 732, 225, 300, 300, 551, 572, 268, 322, 268, 372, 572, 572, 572, 572, 572, 572, 572, 572,
        572, 572, 268, 268, 572, 572, 572, 434, 899, 639, 650, 632, 730, 556, 519, 728, 741, 339, 273, 619, 524, 907, 760, 781,
        605, 781, 622, 549, 556, 731, 600, 930, 586, 566, 572, 329, 372, 329, 572, 444, 281, 561, 615, 480, 615, 564, 344, 615,
        618, 258, 258, 534, 258, 935, 618, 605, 615, 615, 413, 479, 361, 618, 508, 786, 529, 510, 470, 380, 551, 380, 572]
_BOLD = [260, 286, 472, 646, 572, 901, 750, 266, 339, 339, 545, 572, 285, 322, 285, 413, 572, 572, 572, 572, 572, 572, 572, 572,
         572, 572, 285, 285, 572, 572, 572, 477, 897, 690, 672, 637, 740, 560, 549, 724, 765, 389, 331, 664, 565, 943, 813, 796,
         628, 796, 660, 551, 579, 756, 650, 967, 667, 624, 579, 331, 413, 331, 572, 411, 362, 604, 633, 514, 633, 591, 387, 633,
         657, 305, 305, 620, 305, 982, 657, 619, 633, 633, 454, 497, 434, 657, 569, 856, 578, 569, 488, 394, 551, 394, 572]


def width(s, size, mono=False, bold=False):
    if mono:
        return len(s) * size * 0.602
    table = _BOLD if bold else _REG
    return sum(table[ord(c) - 32] if 32 <= ord(c) < 127 else 600 for c in s) * size / 1000


def mix(a, b, t):
    """a over b at t, as a hex colour (CSS color-mix in srgb)."""
    pa = [int(a[i:i + 2], 16) for i in (1, 3, 5)]
    pb = [int(b[i:i + 2], 16) for i in (1, 3, 5)]
    return "#" + "".join(f"{round(x * t + y * (1 - t)):02x}" for x, y in zip(pa, pb))


# ------------------------------------------------------------------ themes
# ui/src/styles/globals.css; agent colours by name for the demo team.
LIGHT = dict(
    page="#f6f7f9", s0="#ffffff", s1="#fafafa", s2="#f0f1f4", s3="#e3e5ea", div="#e3e5ea", div2="#cdd1d8",
    ink="#1f2329", ink2="#424854", muted="#6b7280", faint="#9aa0ac",
    info="#0284c7", ok="#16a34a", warn="#b45309", danger="#dc2626", brand="#fc6d26",
    agents={"coordinator": "#2563eb", "engineer": "#0d9488", "reviewer": "#7c3aed", "tech-lead": "#ea580c"},
    kw="#0000ff", str="#a31515", op="#6b7280", tint=0.18, dark=False)
LIGHT["link"] = mix(LIGHT["info"], LIGHT["ink"], 0.85)
LIGHT["oki"] = mix(LIGHT["ok"], LIGHT["ink"], 0.70)  # success-ink, #197d40: green text
DARK = dict(
    page="#111317", s0="#1a1d23", s1="#16181d", s2="#21252d", s3="#2b2f38", div="#2b2f38", div2="#353a44",
    ink="#e4e6eb", ink2="#b3b7c0", muted="#8b919e", faint="#5e6471",
    info="#38bdf8", ok="#22c55e", warn="#f59e0b", danger="#ef4444", brand="#fc6d26",
    agents={"coordinator": "#60a5fa", "engineer": "#2dd4bf", "reviewer": "#a78bfa", "tech-lead": "#fb923c"},
    kw="#569cd6", str="#ce9178", op="#8b919e", tint=0.22, dark=True)
DARK["link"] = DARK["info"]
DARK["oki"] = DARK["ok"]


class Sheet:
    def __init__(self, w, h, title, sub, t=LIGHT):
        self.w, self.h, self.t, self.out = w, h, t, []
        self.rect(0, 0, w, h, t["page"])
        self.text(40, 46, title, 22, t["ink"], 700)
        self.text(40, 72, sub, 13, t["muted"])

    # primitives
    def rect(self, x, y, w, h, fill="none", stroke=None, r=0, dash=None, sw=1):
        a = f' stroke="{stroke}" stroke-width="{sw}"' if stroke else ""
        a += f' stroke-dasharray="{dash}"' if dash else ""
        self.out.append(f'<rect x="{x:.1f}" y="{y:.1f}" width="{w:.1f}" height="{h:.1f}" rx="{r}" fill="{fill}"{a}/>')

    def line(self, x1, y1, x2, y2, color=None, dash=None):
        d = f' stroke-dasharray="{dash}"' if dash else ""
        self.out.append(f'<line x1="{x1:.1f}" y1="{y1:.1f}" x2="{x2:.1f}" y2="{y2:.1f}" stroke="{color or self.t["div"]}" stroke-width="1"{d}/>')

    def text(self, x, y, s, size=12, fill=None, weight=400, mono=False, anchor="start", underline=False, rotate=None):
        u = ' text-decoration="underline"' if underline else ""
        r = f' transform="rotate({rotate} {x} {y})"' if rotate else ""
        self.out.append(
            f'<text x="{x:.1f}" y="{y:.1f}" font-family="{MONO if mono else SANS}" font-size="{size}" '
            f'font-weight="{weight}" fill="{fill or self.t["ink"]}" text-anchor="{anchor}" xml:space="preserve"{u}{r}>{escape(s)}</text>')
        return x + width(s, size, mono, weight >= 600)

    def path(self, d, color, sw=1.4):
        self.out.append(f'<path d="{d}" fill="none" stroke="{color}" stroke-width="{sw}" stroke-linecap="round" stroke-linejoin="round"/>')

    def arrow(self, x, y, w=12, color=None):
        """A right arrow on the text's middle line, from x."""
        c = color or self.t["muted"]
        self.path(f"M{x},{y} H{x + w} M{x + w - 4},{y - 4} L{x + w},{y} L{x + w - 4},{y + 4}", c)

    def newtab(self, x, y, color):
        """The ↗ after a link that opens a new tab; y is the baseline."""
        self.path(f"M{x + 2},{y - 1} L{x + 8},{y - 7} M{x + 3.5},{y - 7} H{x + 8} V{y - 2.5}", color, 1.3)
        return x + 11

    def focus(self, x, y, w, h):
        self.rect(x, y, w, h, stroke=self.t["brand"], r=2)

    def note(self, x, y, n):
        self.rect(x, y, 18, 18, self.t["ink"], r=2)
        self.text(x + 9, y + 13, str(n), 11, self.t["s0"], 700, anchor="middle")

    def notes(self, x, y, items, title="Notes"):
        self.text(x, y, title, 13, self.t["ink"], 700)
        y += 24
        for n, body in items:
            self.note(x, y - 13, n)
            for i, part in enumerate(body):
                self.text(x + 28, y + 17 * i, part, 12, self.t["ink2"])
            y += 17 * len(body) + 10
        return y

    def label(self, x, y, s):
        self.text(x, y, s, 12, self.t["muted"], 600)

    def mark(self):
        return len(self.out)

    def under(self, mark, x, y, w, h):
        """A panel drawn behind what was drawn since mark, once its height is known."""
        self.out.insert(mark, f'<rect x="{x}" y="{y}" width="{w}" height="{h:.1f}" rx="2" fill="{self.t["s0"]}" '
                              f'stroke="{self.t["div2"]}" stroke-width="1"/>')

    def save(self, name):
        svg = (f'<svg xmlns="http://www.w3.org/2000/svg" width="{self.w}" height="{self.h}" '
               f'viewBox="0 0 {self.w} {self.h}" role="img">' + "".join(self.out) + "</svg>\n")
        with open(os.path.join(OUT, name), "w") as f:
            f.write(svg)

    # pieces of the page
    def chip(self, x, y, who, size=20):
        """Avatar: the first letter on a tint of the agent's colour; neutral for you and Agentry."""
        t = self.t
        c = t["agents"].get(who)
        if c:
            fill, ink = mix(c, t["s1"], t["tint"]), (c if t["dark"] else mix(c, t["ink"], 0.75))
        else:
            fill, ink = t["s3"], t["ink2"]
        self.rect(x, y, size, size, fill, r=3)
        self.text(x + size / 2, y + size * 0.69, who[0].upper(), size * 0.55, ink, 700, True, "middle")

    def badge(self, x, y, label, tone="default"):
        t = self.t
        fg = {"default": t["ink2"], "info": t["info"], "ok": t["ok"], "warn": t["warn"], "danger": t["danger"]}[tone]
        bg = t["s3"] if tone == "default" else mix(fg, t["s0"], 0.12)
        w = 12 + len(label) * 6.6
        self.rect(x, y, w, 18, bg, r=2)
        self.text(x + w / 2, y + 12.5, label.upper(), 10, fg, 500, True, "middle")
        return w

    def name(self, x, y, who, size=12.5, color=None):
        return self.text(x, y, who, size, color or self.t["ink"], 700, True)

    def agent_dot(self, x, y, who):
        """AgentName's 8px dot, centred on y."""
        self.rect(x, y - 4, 8, 8, self.t["agents"].get(who, self.t["faint"]), r=4)

    def runs(self, x, y, parts, size=13):
        """One line of text in runs: (text, kind). Kinds: '' body, b bold, m mono, muted, code (inline code),
        link, linkm (mono link), linkc (inline code that is a link: a mono link, no code background),
        nt (new-tab icon), h heading."""
        t = self.t
        for s, kind in parts:
            if kind == "nt":
                x = self.newtab(x + 1, y, t["link"]) + 1
            elif kind == "code":
                cs = size * 0.88
                w = width(s, cs, True)
                self.rect(x, y - size + 1, w + 8, size + 5, t["s3"], r=2)
                self.text(x + 4, y, s, cs, t["ink"], 400, True)
                x += w + 10
            else:
                fill, weight, mono, u = {
                    "": (t["ink2"], 400, False, False), "b": (t["ink"], 700, False, False),
                    "m": (t["ink"], 400, True, False), "muted": (t["muted"], 400, False, False),
                    "link": (t["link"], 400, False, True), "linkm": (t["link"], 400, True, True),
                    "linkc": (t["link"], 400, True, True), "h": (t["ink"], 700, False, False),
                }[kind]
                sz = size * 0.92 if mono else size
                x = self.text(x, y, s, sz, fill, weight, mono, underline=u)
        return x

    def code_line(self, x, y, toks, plain=False, size=12):
        t = self.t
        for s, k in toks:
            fill = t["ink"] if plain else {"kw": t["kw"], "str": t["str"], "op": t["op"], "": t["ink"]}[k]
            x = self.text(x, y, s, size, fill, 400, True)
        return x

    def code_box(self, x, y, w, lines, plain=False, lang=None, copy=False, size=12):
        t = self.t
        h = 16 + 20 * len(lines)
        self.rect(x, y, w, h, t["s1"], t["div"], 3)
        for i, toks in enumerate(lines):
            self.code_line(x + 10, y + 22 + 20 * i, toks, plain, size)
        rx = x + w - 10
        if copy:
            self.path(f"M{rx - 10},{y + 8} h7 v9 h-7 z M{rx - 7},{y + 6} h7 v9", t["muted"], 1.1)
            rx -= 20
        if lang:
            self.text(rx, y + 17, lang, 11, t["muted"], 400, True, "end")
        return h

    def fold(self, x, y, label, focused=False):
        w = width(label, 12, False, True)
        self.text(x, y, label, 12, self.t["muted"], 600)
        if focused:
            self.focus(x - 4, y - 14, w + 8, 20)
        return w


# Commands, as (text, token kind) runs per line.
BASH_RUN = [[("go", "kw"), (" test ./internal/web/... -run ", ""), ("'TestActivity|TestLinks'", "str")]]
BASH_LONG = [
    [("cd", "kw"), (" ui ", ""), ("&&", "op"), (" npm", "kw"), (" run lint ", ""), ("&&", "op"), (" npm", "kw"), (" run build ", ""), ("&&", "op"),
     (" cd", "kw"), (" .. ", ""), ("&&", "op")],
    [("go", "kw"), (" test ./internal/web/... -run ", ""), ("'TestDist|TestTask'", "str"), (" ", ""), ("&&", "op"), (" git", "kw"),
     (" add internal/web/dist", "")],
    [("ui/src/components/organisms/ActivityFeed.tsx ui/src/lib/links.ts ", ""), ("&&", "op"), (" git", "kw"), (" commit -F -", "")],
]
FENCE_TS = [[("const", "kw"), (" to = linkInto(task, href);", "")],
            [("if", "kw"), (" (!to) ", ""), ("return", "kw"), (" label; ", ""), ("// javascript:, data:", "op")]]
FENCE_SH = [[("cd", "kw"), (" ui ", ""), ("&&", "op"), (" npm", "kw"), (" run lint ", ""), ("&&", "op"), (" npm", "kw"), (" run build", "")],
            [("go", "kw"), (" test ./...", "")]]


class Feed:
    """An Activity feed drawn top down: bands, rows and the time rail."""

    def __init__(self, s, x, y, w, phone=False):
        self.s, self.t, self.x, self.w, self.y, self.phone = s, s.t, x, w, y, phone
        self.rail = 56 if phone else 120
        self.pad = 16 if phone else 24
        self.R = x + w - self.rail  # where the rail column starts
        self.cx = x + self.pad + 28  # where content starts, under the band's name
        self.dot, self.tx = (4, 18) if phone else (10, 28)  # the rail's dot and time, from R
        self.m = s.mark()  # the rail line goes under everything drawn after this

    @property
    def cw(self):
        return self.R - self.cx - 12

    def toolbar(self, rows="214 rows", on="Everything"):
        s, t, x = self.s, self.t, self.x
        s.rect(x, self.y, self.w, 44, t["s0"])
        s.line(x, self.y + 44, x + self.w, self.y + 44)
        bx = x + self.pad
        s.rect(bx, self.y + 9, 228, 26, t["s2"], t["div"], 3)
        for label, mid, sw in (("Everything", 44, 84), ("Actions", 116, 60), ("Messages", 182, 70)):
            if label == on:
                s.rect(bx + mid - sw / 2, self.y + 11, sw, 22, t["s0"], t["div2"], 2)
            s.text(bx + mid, self.y + 26, label, 11.5, t["ink"] if label == on else t["muted"], 600 if label == on else 500, anchor="middle")
        if not self.phone:
            s.text(x + self.w - self.pad, self.y + 27, rows, 11, t["muted"], 400, True, "end")
        self.y += 44

    def rail_line(self, y0, y1):
        self.s.out.insert(self.m, f'<line x1="{self.R + self.dot + 4}" y1="{y0}" x2="{self.R + self.dot + 4}" y2="{y1:.1f}" stroke="{self.t["div"]}" stroke-width="1"/>')

    def stamp(self, y, time, dur=None, live=False, dot=None):
        s, t = self.s, self.t
        s.rect(self.R + self.dot, y - 9, 9, 9, dot or (t["ok"] if live else t["faint"]), r=4.5)
        s.text(self.R + self.tx, y, time, 11, t["muted"], 400, True)
        if dur and not self.phone:
            s.text(self.R + self.tx, y + 14, dur, 11, t["oki"] if live else t["muted"], 400, True)

    def band(self, who, team="", turn="", status=None, tokens="", time="", dur="", live=False, continued=False, under=None):
        """An author band: chip, name, team and role, then the turn's facts. under: draw it as stuck."""
        s, t, x, y = self.s, self.t, self.x, self.y
        s.rect(x + 1, y, self.w - 2, 44, t["s1"])
        s.line(x, y, x + self.w, y)
        s.line(x, y + 44, x + self.w, y + 44)
        px = x + self.pad
        s.chip(px, y + 12, who)
        nx = s.name(px + 28, y + 27, who) + 10
        if self.phone:
            if status:
                bx = nx + 2
                bx += s.badge(bx, y + 13, *status) + 8
                if dur:
                    s.text(bx, y + 27, dur, 11, t["oki"] if live else t["muted"], 400, True)
        else:
            if team:
                nx = s.text(nx, y + 27, team, 12, t["muted"]) + 14
            if turn:
                nx = s.text(nx, y + 27, turn, 11, t["muted"], 400, True) + 8
            if continued:
                nx = s.text(nx, y + 27, "continued", 11.5, t["muted"]) + 10
            if status:
                nx += s.badge(nx + 2, y + 13, *status) + 12
            if tokens:
                s.text(nx, y + 27, tokens, 11, t["muted"], 400, True)
        if time:
            self.stamp(y + 22, time, dur, live)
        self.y += 44
        return y

    def icon(self, y, kind="tool"):
        s, t = self.s, self.t
        ix = self.x + self.pad + 2
        if kind == "note":
            s.rect(ix + 6, y - 5, 4, 4, t["faint"], r=2)
        elif kind == "msg":
            s.path(f"M{ix + 1},{y - 11} h13 v9 h-8 l-3,3 v-3 h-2 z", t["faint"], 1.2)
        elif kind == "done":
            s.rect(ix + 1, y - 12, 13, 13, stroke=t["ok"], r=6.5)
            s.path(f"M{ix + 4.5},{y - 5.5} l2.5,2.5 l4,-5", t["ok"], 1.3)
        else:
            s.path(f"M{ix + 3},{y - 1} l6,-6 m-1,-4 a3,3 0 1,0 4,4", t["faint"], 1.3)

    def note_row(self, text, time):
        y = self.y + 20
        self.icon(y, "note")
        self.s.text(self.cx, y, text, 12, self.t["muted"])
        self.stamp(y, time)
        self.y += 30

    def action(self, lines, badge, time, dur="", live=False, fold=None, plain=False, tool="Bash", box_w=None, note=None):
        s, t = self.s, self.t
        y = self.y + 20
        self.icon(y)
        s.text(self.cx, y, tool, 11, t["muted"], 400, True)
        bw = box_w or (self.cw - (0 if self.phone else 100))
        h = s.code_box(self.cx, y + 8, bw, lines, plain)
        if self.phone:
            by = y + 8 + h + 8
            bw2 = s.badge(self.cx, by, *badge)
            if fold:
                s.fold(self.cx + bw2 + 14, by + 13, fold)
                fold = None
            end = by + 18
        else:
            s.badge(self.cx + bw + 12, y + 10, *badge)
            end = y + 8 + h
        if fold:
            s.fold(self.cx, end + 20, fold)
            end += 22
        self.stamp(y, time, dur, live, t["ok"] if live else None)
        if note:
            s.note(*note(y, h))
        self.y = end + 14

    def message(self, frm, to, leads=None):
        s, t = self.s, self.t
        y = self.y + 22
        self.icon(y, "msg")
        x = self.cx
        s.chip(x, y - 13, frm, 18)
        x = s.name(x + 24, y, frm, 12.5) + 6
        s.arrow(x, y - 4)
        x += 20
        s.chip(x, y - 13, to, 18)
        x = s.name(x + 24, y, to, 12.5) + 10
        if leads:
            s.text(x, y, leads, 11, t["muted"], 400, True)
        self.y = y + 10
        return y

    def body(self, blocks, size=13, lh=21):
        """Compact Markdown: ('p', runs), ('li', runs), ('ol', n, runs), ('h', text), ('code', lines, lang, plain),
        ('table', rows), ('gap',), ('pi', runs) for a list item's next line."""
        s, t = self.s, self.t
        y = self.y
        for b in blocks:
            if b[0] == "p":
                s.runs(self.cx, y + 16, b[1], size)
                y += lh
            elif b[0] == "pi":  # a list item's wrapped line
                s.runs(self.cx + 18, y + 16, b[1], size)
                y += lh
            elif b[0] == "h":
                s.text(self.cx, y + 17, b[1], 14, t["ink"], 700)
                y += lh + 4
            elif b[0] == "li":
                s.rect(self.cx + 6, y + 10, 4, 4, t["ink2"], r=2)
                s.runs(self.cx + 18, y + 16, b[1], size)
                y += lh
            elif b[0] == "ol":
                s.text(self.cx + 2, y + 16, f"{b[1]}.", size, t["ink2"])
                s.runs(self.cx + 18, y + 16, b[2], size)
                y += lh
            elif b[0] == "code":
                h = s.code_box(self.cx, y + 6, b[4] if len(b) > 4 else self.cw - 40, b[1], b[3], b[2], copy=True)
                y += h + 12
            elif b[0] == "table":
                cols = b[2]
                tx = self.cx
                for r, row in enumerate(b[1]):
                    ry = y + 4 + 26 * r
                    if r == 0:
                        s.rect(tx, ry, sum(cols), 26, t["s1"])
                    s.rect(tx, ry, sum(cols), 26, stroke=t["div"])
                    cx = tx
                    for c, cell in enumerate(row):
                        if c:
                            s.line(cx, ry, cx, ry + 26)
                        s.runs(cx + 8, ry + 17, cell if isinstance(cell, list) else [(cell, "b" if r == 0 else "")], 12.5)
                        cx += cols[c]
                y += 26 * len(b[1]) + 12
            elif b[0] == "gap":
                y += b[1]
        self.y = y
        return y

    def fold_line(self, label, focused=False, cut=True):
        """The clean edge under a folded entry, then its button."""
        s, t = self.s, self.t
        if cut:
            s.line(self.cx, self.y + 4, self.R - 24, self.y + 4)
        s.fold(self.cx, self.y + 24, label, focused)
        self.y += 38

    def end(self, pad=10):
        self.y += pad


# ------------------------------------------------------------------ the demo feed
REPORT = [
    ("h", "Done"),
    ("p", [("Lint, build and ", ""), ("go test ./...", "code"), (" pass. Committed ", ""), ("3f2a9c1", "linkc"), ("nt", "nt"),
           (" on top of ", ""), ("9e1d04b", "code"), (" from dev.", "")]),
    ("li", [("Author bands in ", ""), ("ActivityFeed.tsx", "linkm"), ("nt", "nt"), (", links in ", ""), ("lib/links.ts", "linkm"), ("nt", "nt")]),
    ("li", [("Same fix as ", ""), ("task-21", "linkm"), (" did for the chat; upstream: ", ""), ("github.com/…/issues/88", "link"), ("nt", "nt")]),
    ("li", [("A ", ""), ("javascript:", "code"), (" link reads as text: Open the demo", "")]),
]
REPORT_TABLE = ("table", [["Check", "Result"], ["npm run lint", "pass"], ["go test ./...", "pass"]], [180, 120])
BRIEF = [
    ("h", "Build the author bands"),
    ("p", [("Read ", ""), ("spec.md", "linkm"), ("nt", "nt"), (" and ", ""), ("the flows", "link"), ("nt", "nt"), (" first. Then:", "")]),
    ("ol", 1, [("Render briefs and reports with ", ""), ("Markdown", "code"), (" in compact mode", "")]),
    ("ol", 2, [("Colour commands lazily, as ", ""), ("MonacoColorize", "code"), (" does", "")]),
    ("ol", 3, [("Check it at 390 wide:", "")]),
]
REPORT_PHONE = [
    ("h", "Done"),
    ("p", [("Lint, build, ", ""), ("go test", "code"), (" pass.", "")]),
    ("p", [("Committed ", ""), ("3f2a9c1", "linkc"), ("nt", "nt"), (" on ", ""), ("9e1d04b", "code")]),
    ("li", [("Bands in ", ""), ("ActivityFeed.tsx", "linkm"), ("nt", "nt")]),
    ("li", [("Like ", ""), ("task-21", "linkm"), ("; see ", ""), ("issue 88", "link"), ("nt", "nt")]),
]
BASH_PHONE = [
    [("cd", "kw"), (" ui ", ""), ("&&", "op"), (" npm", "kw"), (" run lint ", ""), ("&&", "op")],
    [("npm", "kw"), (" run build ", ""), ("&&", "op"), (" cd", "kw"), (" .. ", ""), ("&&", "op")],
    [("go", "kw"), (" test ./internal/web/...", "")],
]


def desktop_feed(s, x, y, w, notes=True, length="long"):
    """The 1280-wide feed fragment; returns its Feed. notes: draw the numbered markers."""
    f = Feed(s, x, y, w)
    f.toolbar()
    top = f.y
    t = s.t
    n = (lambda *a: s.note(*a)) if notes else (lambda *a: None)

    # 1. Scrolled: a row half under the stuck band, then the band on top of it.
    f.y = top
    f.action(BASH_LONG[:1], ("done",), "06:16")
    below = f.y
    f.y = top
    f.band("reviewer", "Software", "turn-6", ("running", "info"), "18k tokens", "05:58", "18m…", live=True, continued=True)
    n(x + w - 150, f.y - 31, 1)
    f.y = below
    f.action(BASH_RUN, ("running", "info"), "06:15", "1m…", live=True)
    f.action(BASH_LONG, ("done",), "06:13", "40s", fold="Show all · 9 lines")
    n(f.cx + 140, f.y - 45, 4)

    # 2. You, in the middle of the reviewer's turn.
    f.band("you", time="06:02")
    n(x + w - 150, f.y - 31, 2)
    f.message("you", "reviewer")
    f.body([("p", [("Check the links at 390 wide too, as ", ""), ("the spec", "link"), ("nt", "nt"), (" says.", "")])])
    f.end(6)
    f.stamp(f.y - 36, "06:02")

    # 3. The reviewer's turn where it started.
    f.band("reviewer", "Software", "turn-6", ("running", "info"), "", "05:58")
    f.note_row("Read its brief", "05:58")

    # 4. Agentry's own rows between turns.
    f.band("Agentry", time="05:57")
    n(x + w - 150, f.y - 31, 3)
    f.note_row("Held in Review", "05:57")
    f.note_row("Worktree ready on agentry/task-40", "05:57")

    # 5. The engineer's turn: a report, folded.
    f.band("engineer", "Software", "turn-5", ("ended",), "41k tokens", "05:09", "48m")
    ry = f.y + 20
    f.icon(ry, "done")
    s.text(f.cx, ry, "Finished, 41k tokens", 12, t["oki"], 600)
    f.stamp(ry, "05:57")
    f.y = ry + 4
    if length == "short":
        f.body(REPORT)
        n(x + w - 150, ry + 8, 5)
        return f, top
    # Folded at 240px (about 10 lines), the cut snapped up to the last whole line: here after
    # the code box's first line. Everything below it is clipped.
    cut = f.y
    i = len(s.out)
    f.body(REPORT)
    n(x + w - 150, ry + 8, 5)
    f.body([REPORT_TABLE])
    fence = f.y
    f.body([("code", FENCE_TS, "ts", False, 420)])
    h = fence + 6 + 30 - cut
    s.out.insert(i, f'<clipPath id="fold5"><rect x="{x}" y="{cut}" width="{w}" height="{h}"/></clipPath><g clip-path="url(#fold5)">')
    s.out.append("</g>")
    n(f.cx + 430, fence + 8, 6)
    f.y = cut + h
    f.fold_line("Show all · 38 lines")
    n(f.cx + 140, f.y - 28, 7)
    f.action(BASH_RUN, ("done",), "05:41", "2m")

    # 6. The coordinator's turn: a brief, opened.
    f.band("coordinator", "Software", "turn-4", ("ended",), "8k tokens", "05:00", "9m")
    my = f.message("coordinator", "engineer", "turn-5")
    n(f.cx + 360, my - 13, 10)
    f.stamp(my, "05:08")
    f.body(BRIEF)
    f.body([("code", FENCE_SH, "bash", True, 420)])
    n(f.cx + 430, f.y - 58, 8)
    f.body([("ol", 4, [("Hand it back with a report, not a question", "")])])
    f.fold_line("Show less", focused=True, cut=False)
    n(f.cx + 90, f.y - 28, 9)

    # 7. You, at the start.
    f.band("you", time="04:58")
    my = f.message("you", "coordinator")
    f.stamp(my, "04:58")
    end = f.body([("p", [("Make Activity and Handoffs readable. Unsafe: ", ""), ("click me", "")])])
    n(f.cx + 400, end - 17, 11)
    f.end(8)
    return f, top


def activity_desktop():
    s = Sheet(1280, 1660, "Activity · Author bands · 1280 wide",
              "Newest at the top. Scrolled a little: the reviewer's band is stuck, rows pass under it, and every row on screen sits under a band that names its author.")
    X, Y, W = 40, 120, 820
    s.label(X, Y - 10, "ACTIVITY  ·  task-40  ·  scrolled")
    m = s.mark()
    f, top = desktop_feed(s, X, Y, W)
    f.rail_line(top, f.y + 10)
    s.under(m, X, Y, W, f.y - Y + 10)
    # redraw the bands over the rail line? The rail is drawn last on purpose: it runs under dots only.
    # The screen: 1280×800 is about 700px of feed under the page header.
    s.path(f"M{X - 14},{Y + 44} h6 M{X - 11},{Y + 44} V{Y + 757} M{X - 14},{Y + 757} h6", s.t["muted"], 1)
    s.text(X - 18, Y + 394, "on screen at 1280 × 800", 11, s.t["muted"], 500, anchor="middle", rotate=-90)
    s.line(X, Y + 757, X + W, Y + 757, s.t["muted"], "4 4")
    s.rect(X + W - 236, Y + 759, 228, 18, s.t["s0"])
    s.text(X + W - 12, Y + 772, "end of the screen: below, code is plain", 11, s.t["muted"], 400, anchor="end")

    s.notes(900, 130, [
        (1, ["Stuck band: the reviewer's turn continues", "here, after your message. ‘continued’ says", "its true start is further down. Rows scroll", "under it; it is pushed out by the next band.", "Band: chip, name, team · role (role dropped", "when it is the name), turn, status, tokens.", "An h3 holding a button: narrows to this turn.", "Always one line, 44px, stuck or not."]),
        (2, ["You, in the middle of a running turn: a", "short band of its own (neutral chip), the", "message ‘you → reviewer’, then the turn", "resumes below. Not a button (no turn)."]),
        (3, ["Agentry's own rows between turns (held,", "worktree ready, files fetched): a band", "with a neutral chip, no turn facts."]),
        (4, ["Commands: plain mono at once, coloured by", "Monaco once near the screen. Wrap at spaces,", "3 lines, then ‘Show all · 9 lines’. No", "sideways scroll. Badge right; under on a phone."]),
        (5, ["Links: underlined always; ↗ = new tab.", "3f2a9c1 is a commit of the branch: a mono", "link with no code background (5.0:1). 9e1d04b", "isn't: plain code. task-21: same tab. Files:", "the Workspace preview."]),
        (6, ["Fenced code: same box, language and Copy", "top right (44px hit area), coloured by", "language. Long lines scroll inside the box."]),
        (7, ["Folded at 240px (about 10 lines), clip, clean", "edge, no fade; here the cut falls in a code box.", "Links below the cut leave the Tab order", "(tabindex -1) until Show all. 38 is about:", "measured height ÷ line height, not exact."]),
        (8, ["Below the screen's fold: the same box in", "plain mono. It colours when it comes within", "200px of view; nothing moves when it does."]),
        (9, ["Opened: ‘Show less’ under the last line,", "shown with keyboard focus (the global 1px", "brand outline). Closing scrolls the entry's", "top back under the band if it went above."]),
    ])
    s.notes(900, 1280, [
        (10, ["The leads-to turn-5 is a button with a 24 × 24", "hit box (padding overlaps the row's)."]),
        (11, ["‘click me’ was [click me](javascript:…): an", "unsafe link reads as plain text, no hint."]),
        (12, ["Rail: start time and duration in ink-muted", "(was ink-faint, 2.6:1). A live duration ends", "in … and is success-ink #197d40 (5.2:1)."]),
        (13, ["scroll-padding-top: 44px on the scroller (the", "band's fixed height), so focus never hides", "under the band. Newest: bottom right."]),
    ], "Also")
    s.save("activity-desktop.svg")


# ------------------------------------------------------------------ phone
def phone_frame(s, x, y, title, h=780, tab="Activity"):
    s.label(x, y - 10, title)
    s.rect(x, y, 390, h, s.t["s0"], s.t["div2"], 4)
    # the page's tab strip above the feed
    s.rect(x + 1, y + 1, 388, 40, s.t["s0"])
    s.line(x, y + 41, x + 390, y + 41)
    tx = x + 16
    for name in ("Chat", "Activity", "Handoffs", "Files"):
        on = name == tab
        tx2 = s.text(tx, y + 26, name, 13, s.t["ink"] if on else s.t["muted"], 600 if on else 500)
        if on:
            s.rect(tx, y + 38, tx2 - tx, 3, s.t["ink"])
        tx = tx2 + 22
    return y + 42


def activity_mobile():
    s = Sheet(1280, 1000, "Activity · Author bands · 390 wide",
              "Same feed on a phone: the band keeps chip, name and status; the rail is time alone; badges go under commands; nothing scrolls the page sideways.")
    t = s.t
    # Phone 1: scrolled, band stuck.
    y = phone_frame(s, 40, 130, "SCROLLED, BAND STUCK")
    f = Feed(s, 40, y, 390, phone=True)
    f.toolbar()
    top = f.y
    f.band("reviewer", status=("running", "info"), time="05:58", dur="18m…", live=True)
    s.note(40 + 390 - 84, f.y - 31, 1)
    f.action([[("go", "kw"), (" test ./internal/web/...", "")], [("-run ", ""), ("'TestActivity|TestLinks'", "str")]],
             ("running", "info"), "06:15", live=True)
    f.action(BASH_PHONE, ("done",), "06:13", fold="Show all · 9 lines")
    s.note(40 + 300, f.y - 30, 2)
    f.band("you", time="06:02")
    my = f.message("you", "reviewer")
    f.stamp(my, "06:02")
    f.body([("p", [("Check the links at 390 too, as ", ""), ("the spec", "link"), ("nt", "nt")]),
            ("p", [("says.", "")])])
    f.end(4)
    f.band("reviewer", status=("running", "info"), time="05:58")
    f.note_row("Read its brief", "05:58")
    f.band("Agentry", time="05:57")
    f.note_row("Held in Review", "05:57")
    f.note_row("Worktree ready", "05:57")
    f.band("engineer", status=("ended",), time="05:09", dur="48m")
    f.note_row("Read its brief", "05:09")
    f.rail_line(top, f.y)
    s.note(40 + 300, f.y - 180, 3)
    # Newest, once scrolled 200px: bottom right of the scroller, clear of every band's name.
    bw, bh = 92, 32
    bx, by = 40 + 390 - 16 - bw, 130 + 780 - 16 - bh
    s.rect(bx, by, bw, bh, t["s2"], t["div2"], 3)
    s.path(f"M{bx + 16},{by + 21} V{by + 11} M{bx + 12},{by + 15} L{bx + 16},{by + 11} L{bx + 20},{by + 15}", t["ink"], 1.4)
    s.text(bx + 28, by + 21, "Newest", 12, t["ink"], 500)
    s.note(bx - 26, by + 7, 7)

    # Phone 2: a report opened, links, a table that scrolls in its box.
    y = phone_frame(s, 470, 130, "A REPORT, OPENED")
    f = Feed(s, 470, y, 390, phone=True)
    f.toolbar()
    top = f.y
    # a band that isn't stuck is the same one line: no second line to lose when it sticks
    by = f.band("engineer", status=("ended",), time="05:09", dur="48m")
    s.note(470 + 270, by + 13, 4)
    ry = f.y + 20
    f.icon(ry, "done")
    s.text(f.cx, ry, "Finished, 41k tokens", 12, t["oki"], 600)
    f.stamp(ry, "05:57")
    f.y = ry + 4
    f.body(REPORT_PHONE)
    s.note(470 + 340, f.y - 70, 5)
    # a table wider than the column: it scrolls in its own box
    tx, ty = f.cx, f.y + 6
    clip = f.cw + 6
    s.out.append(f'<clipPath id="tbl"><rect x="{tx}" y="{ty}" width="{clip}" height="80"/></clipPath><g clip-path="url(#tbl)">')
    cols = [130, 70, 150]
    for r, row in enumerate([["Check", "Result", "Where"], ["npm run lint", "pass", "ui/"], ["go test ./...", "pass", "internal/web"]]):
        yy = ty + 26 * r
        if r == 0:
            s.rect(tx, yy, sum(cols), 26, t["s1"])
        s.rect(tx, yy, sum(cols), 26, stroke=t["div"])
        cx = tx
        for c, cell in enumerate(row):
            if c:
                s.line(cx, yy, cx, yy + 26)
            s.text(cx + 8, yy + 17, cell, 12.5, t["ink"] if r == 0 else t["ink2"], 700 if r == 0 else 400)
            cx += cols[c]
    s.out.append("</g>")
    s.line(tx + clip, ty, tx + clip, ty + 92, t["div2"])  # the scroll box's edge: the Where column is cut here
    s.rect(tx + 2, ty + 84, clip - 4, 6, t["s2"], r=3)
    s.rect(tx + 2, ty + 84, (clip - 4) * 0.62, 6, t["muted"], r=3)
    f.y = ty + 100
    s.note(470 + 340, ty + 30, 6)
    f.body([("code", [[("npm", "kw"), (" run build ", ""), ("&&", "op"), (" go", "kw"), (" test", "")]],
             "bash", False, f.cw)])
    f.fold_line("Show less", cut=False)
    f.band("coordinator", status=("ended",), time="05:00", dur="9m")
    my = f.message("coordinator", "engineer")
    f.stamp(my, "05:08")
    f.body([("h", "Build the author bands"),
            ("p", [("Read ", ""), ("spec.md", "linkm"), ("nt", "nt"), (" first. Then:", "")]),
            ("ol", 1, [("Render briefs and reports with", "")]),
            ("pi", [("Markdown", "code"), (" in compact mode", "")]),
            ("ol", 2, [("Colour commands lazily, as", "")]),
            ("pi", [("MonacoColorize", "code"), (" does", "")]),
            ("ol", 3, [("Same link rules as ", ""), ("task-21", "linkm"), (" for", "")]),
            ("pi", [("the chat", "")]),
            ("ol", 4, [("Fold long entries at about 10", "")]),
            ("pi", [("lines", "")])], lh=20)
    f.fold_line("Show all · 38 lines")
    s.note(470 + 340, f.y - 60, 8)
    f.rail_line(top, f.y)

    s.notes(900, 130, [
        (1, ["Every band on a phone is one line, 44px,", "stuck or not: chip, name, status badge,", "duration. Team, role, turn id and tokens", "stay in its accessible name and come back", "at md (768). Names past 120px truncate.", "scroll-padding-top: 44px, as on desktop."]),
        (2, ["Commands wrap to the column. The badge", "drops under the box, left; ‘Show all’ sits", "beside it. 44px hit area on both."]),
        (3, ["Rail: 56px, time only. Durations move into", "the band. Bands still name every author:", "you, Agentry, each agent."]),
        (4, ["Not stuck, the same one line. Nothing", "changes when a band sticks, so no script", "and no jump between two heights."]),
        (5, ["Links wrap with the text; their underline", "and ↗ go with them. Inline links in", "sentences are exempt from target size."]),
        (6, ["A table wider than the column scrolls in", "its own box: the Where column is cut at", "the box's edge, a thin scrollbar under it.", "The page never scrolls sideways. Fences too."]),
        (7, ["Newest (after 200px of scroll) sits at the", "bottom right of the scroller, 16px in, 32px", "tall with a 44px hit area; never over a", "band's author. Was top centre. It covers", "the last ~40px of a line here, so the", "scroller has scroll-padding-bottom: 48px:", "Tab stops a link or Copy above it."]),
        (8, ["The brief folds at about 10 lines, as", "every brief does: ‘Show all · 38 lines’."]),
    ])
    s.save("activity-mobile.svg")


# ------------------------------------------------------------------ handoffs
def card_header(s, x, y, w, frm, to, time, stuck=False):
    """The card's author header: 40px, from → to and the time, a hairline under it. Sticky inside its card."""
    t = s.t
    s.rect(x + 1, y + 1, w - 2, 39, t["s0"])
    s.line(x, y + 40, x + w, y + 40)
    if stuck:
        s.line(x, y, x + w, y)
    cx, hy = x + 12, y + 25
    s.chip(cx, hy - 13, frm, 18)
    nx = s.name(cx + 24, hy, frm, 12.5) + 6
    s.arrow(nx, hy - 4)
    nx += 20
    s.chip(nx, hy - 13, to, 18)
    s.name(nx + 24, hy, to, 12.5)
    s.text(x + w - 12, hy, time, 11, t["muted"], 400, True, "end")


def card(s, x, y, w, frm, to, time, title, body, footer, fold=None, phone=False, focused=False, you=False):
    """A handoff card on the timeline; returns its bottom."""
    t = s.t
    s.rect(x - 23, y + 13, 11, 11, t["muted"] if you else t["agents"].get(frm, t["faint"]), r=5.5)
    pad = 12
    cx = x + pad
    card_header(s, x, y, w, frm, to, time)
    yy = y + 44
    if title:
        s.text(cx, yy + 16, title, 14, t["ink"], 700)
        yy += 26
    for kind, runs in body:
        if kind == "li":
            s.rect(cx + 6, yy + 10, 4, 4, t["ink2"], r=2)
            s.runs(cx + 18, yy + 16, runs, 13)
        elif kind == "ol":
            s.text(cx + 2, yy + 16, runs[0], 13, t["ink2"])
            s.runs(cx + 18, yy + 16, runs[1], 13)
        else:
            s.runs(cx, yy + 16, runs, 13)
        yy += 21
    if fold:
        s.line(cx, yy + 4, x + w - pad, yy + 4)
        s.fold(cx, yy + 24, fold)
        yy += 34
    if footer:
        yy += 6
        s.line(x, yy, x + w, yy)
        fx = cx
        fy = yy + 23
        fx = s.text(fx, fy, footer[0], 11.5, t["muted"]) + 8
        fx = s.text(fx, fy, footer[1], 11, t["ink2"], 400, True)
        fx = s.text(fx, fy, " · ", 11.5, t["muted"])
        fx = s.text(fx, fy, footer[2], 11, t["ink2"], 400, True)
        # the running badge stays at every width; duration and counts only from md
        if len(footer) > 4:
            fx = s.text(fx, fy, " · ", 11.5, t["muted"])
            fx += s.badge(fx, fy - 13, *footer[4])
        if not phone and footer[3]:
            fx = s.text(fx, fy, " · ", 11.5, t["muted"])
            s.text(fx, fy, footer[3], 11.5, t["muted"])
        ax = x + w - pad
        s.path(f"M{ax - 4},{fy - 8} l4,4 l-4,4", t["muted"], 1.3)
        s.text(ax - 10, fy, "Activity", 11.5, t["ink2"], 500, anchor="end")
        if focused:
            s.focus(x + 1, yy + 1, w - 2, 34)
        yy += 36
    elif title or body:
        yy += 10
    else:
        yy = y + 40  # header only
    s.rect(x, y, w, yy - y, stroke=t["div"], r=3)
    return yy


def handoffs():
    s = Sheet(1280, 1700, "Handoffs · Author bands",
              "Each handoff names both people in text with chips and an arrow; the brief is Markdown, folded; the footer, not the card, opens the turn.")
    t = s.t
    X, Y, W = 40, 130, 740
    s.label(X, Y - 10, "HANDOFFS  ·  1280 wide")
    s.rect(X, Y, W, 780, t["s0"], t["div2"], 2)
    lx = X + 29
    s.line(lx, Y + 30, lx, Y + 760)
    # working now
    s.rect(lx - 5, Y + 22, 11, 11, t["ok"], r=5.5)
    s.agent_dot(X + 48, Y + 28, "reviewer")
    nx = s.text(X + 60, Y + 32, "reviewer", 12.5, t["oki"], 700, True)
    s.text(nx + 6, Y + 32, "is working now · watch the terminal", 12.5, t["oki"])
    cx, cw = X + 46, W - 46 - 20
    y = Y + 52
    y = card(s, cx, y, cw, "coordinator", "reviewer", "05:58", "Review the author bands",
             [("p", [("The engineer's report: ", ""), ("3f2a9c1", "linkc"), ("nt", "nt"), (". Check against ", ""), ("flows.md", "linkm"), ("nt", "nt")]),
              ("li", [("Links: none of ", ""), ("javascript:", "code"), (" or ", ""), ("data:", "code"), (" may link", "")]),
              ("li", [("Monaco loads only with code on screen", "")])],
             ("Led to", "turn-6", "reviewer", "6 actions", ("running", "info")), focused=False) + 16
    s.note(X + W + 8, Y + 62, 1)
    y0 = y
    y = card(s, cx, y, cw, "coordinator", "engineer", "05:08", "Build the author bands",
             [("p", [("Read ", ""), ("spec.md", "linkm"), ("nt", "nt"), (" and ", ""), ("the flows", "link"), ("nt", "nt"), (" first. Then:", "")]),
              ("ol", ("1.", [("Render briefs and reports with ", ""), ("Markdown", "code"), (" in compact mode", "")])),
              ("ol", ("2.", [("Colour commands lazily, as ", ""), ("MonacoColorize", "code"), (" does", "")])),
              ("ol", ("3.", [("Same link rules as ", ""), ("task-21", "linkm"), (" for the chat", "")]))],
             ("Led to", "turn-5", "engineer", "48m · 14 actions · 41k tokens"), fold="Show all · 38 lines", focused=True) + 16
    s.note(X + W + 8, y0 + 52, 2)
    s.note(X + 200, y - 72, 3)
    s.note(X + W + 8, y - 44, 4)
    y0 = y
    y = card(s, cx, y, cw, "engineer", "coordinator", "05:57", None,
             [("p", [("Done. Lint, build and tests pass; see ", ""), ("the report", "link"), (" in Activity.", "")])],
             ("From", "turn-5", "engineer", "48m · 14 actions")) + 16
    s.note(X + W + 8, y0 + 10, 5)
    y0 = y
    y = card(s, cx, y, cw, "you", "coordinator", "04:58", None,
             [("p", [("Make Activity and Handoffs readable, like ", ""), ("task-21", "linkm"), (" did for the chat.", "")])],
             ("Led to", "turn-4", "coordinator", "9m · 3 actions"), you=True) + 16
    y0 = y
    y = card(s, cx, y, cw, "coordinator", "you", "05:03", None, [], None)
    s.note(X + W + 8, y0 + 10, 6)

    # Phone
    PX = 820
    py = phone_frame(s, PX, 130, "390 WIDE", 720, "Handoffs")
    plx = PX + 16 + 5
    s.line(plx, py + 20, plx, py + 650)
    pcx, pcw = PX + 40, 390 - 40 - 16
    yy = py + 16
    yy = card(s, pcx, yy, pcw, "coordinator", "reviewer", "05:58", None,
              [("p", [("Review the author bands against ", ""), ("flows.md", "linkm"), ("nt", "nt")])],
              ("Led to", "turn-6", "reviewer", "", ("running", "info")), phone=True) + 14
    s.note(PX + 390 + 8, py + 20, 10)
    yy = card(s, pcx, yy, pcw, "coordinator", "engineer", "05:08", "Build the author bands",
              [("p", [("Read ", ""), ("spec.md", "linkm"), ("nt", "nt"), (" and ", ""), ("the flows", "link"), ("nt", "nt")]),
               ("p", [("first. Then:", "")]),
               ("ol", ("1.", [("Render briefs with ", ""), ("Markdown", "code")])),
               ("ol", ("2.", [("Colour commands lazily", "")]))],
              ("Led to", "turn-5", "engineer", ""), fold="Show all · 38 lines", phone=True) + 14
    s.note(PX + 390 + 8, py + 150, 7)
    yy = card(s, pcx, yy, pcw, "tech-lead", "engineer", "04:02", "Split the work in two",
              [("p", [("Links first, then the bands.", "")])], ("Led to", "turn-3", "engineer", ""), phone=True) + 14
    yy = card(s, pcx, yy, pcw, "you", "coordinator", "03:40", None,
              [("p", [("Like ", ""), ("task-21", "linkm"), (" did for the chat.", "")])], ("Led to", "turn-1", "coordinator", ""), phone=True, you=True)
    assert yy < 130 + 710, yy

    # Scrolled into a long brief, opened: the card's header is stuck at the top of the scroller.
    SY, SH = 980, 300
    s.label(X, SY - 10, "SCROLLED INTO AN OPENED BRIEF  ·  1280 wide")
    s.rect(X, SY, W, SH, t["s0"], t["div2"], 2)
    s.out.append(f'<clipPath id="stuck"><rect x="{X}" y="{SY}" width="{W}" height="{SH}"/></clipPath><g clip-path="url(#stuck)">')
    s.line(lx, SY, lx, SY + SH)
    by = SY + 40 + 6
    for kind, runs in [
        ("ol", ("4.", [("Fold long entries at about 10 lines; links below the cut leave the Tab order", "")])),
        ("ol", ("5.", [("Link ", ""), ("task-21", "linkm"), (", commits like ", ""), ("3f2a9c1", "linkc"), ("nt", "nt"), (" and repo files", "")])),
        ("ol", ("6.", [("Run the checks before you hand it back:", "")])),
    ]:
        s.text(cx + 14, by + 16, runs[0], 13, t["ink2"])
        s.runs(cx + 30, by + 16, runs[1], 13)
        by += 21
    s.code_box(cx + 12, by + 6, 420, FENCE_SH, lang="bash", copy=True)
    by += 16 + 40 + 12
    s.text(cx + 12, by + 16, "Hand it back with a report, not a question.", 13, t["ink2"])
    by += 21
    s.fold(cx + 12, by + 24, "Show less")
    by += 38
    s.line(cx, by, cx + cw, by)
    fy = by + 23
    fx = s.text(cx + 12, fy, "Led to", 11.5, t["muted"]) + 8
    fx = s.text(fx, fy, "turn-5", 11, t["ink2"], 400, True)
    fx = s.text(fx, fy, " · ", 11.5, t["muted"])
    fx = s.text(fx, fy, "engineer", 11, t["ink2"], 400, True)
    s.text(fx, fy, " · 48m · 14 actions · 41k tokens", 11.5, t["muted"])
    s.path(f"M{cx + cw - 16},{fy - 8} l4,4 l-4,4", t["muted"], 1.3)
    s.text(cx + cw - 22, fy, "Activity", 11.5, t["ink2"], 500, anchor="end")
    by += 36
    s.rect(cx, SY - 20, cw, by - SY + 20, stroke=t["div"], r=3)
    nxt = by + 16
    s.rect(cx - 23, nxt + 13, 11, 11, t["agents"]["engineer"], r=5.5)
    card(s, cx, nxt, cw, "engineer", "coordinator", "05:57", None,
         [("p", [("Done. Lint, build and tests pass.", "")])], None)
    card_header(s, cx, SY, cw, "coordinator", "engineer", "05:08", stuck=True)
    s.out.append("</g>")
    s.note(X + W + 8, SY + 11, 8)
    s.note(X + W + 8, SY + by - SY - 30, 9)

    NY = SY + SH + 50
    left = [
        (1, ["Header: from chip and name → to chip and name, both", "in text; the time in ink-muted mono (was faint). 40px,", "a hairline under it. While a turn runs, the footer", "shows its badge and counts, no duration (copy.md)."]),
        (2, ["The brief's first heading becomes the card's title", "and isn't repeated; the rest is compact Markdown."]),
        (3, ["Folded at 8 lines: ‘Show all · 38 lines’. Links work:", "the card is an article, not a button."]),
        (4, ["The footer is the button that opens the turn's", "Activity: 36px row, 44px hit area; shown focused."]),
        (5, ["Said from a turn: ‘From turn-5’.", "6: said to you, no turn: no footer, nothing to open."]),
    ]
    right = [
        (7, ["Phone: from → to and the time always on one line;", "names truncate at 96px each, full names in the", "card's accessible name. The footer keeps the agent:", "‘Led to turn-5 · engineer · Activity ›’."]),
        (8, ["Stuck: the header is position: sticky; top: 0 inside", "its own article, same push rule as Activity's bands.", "The text under it always has an author on screen.", "The scroller's scroll-padding-top is 40px."]),
        (9, ["The card's end pushes its header out; the next", "card's header takes the top. They never overlap.", "You: a neutral slate dot and chip, never a colour."]),
        (10, ["Phone, running: the badge stays,", "‘Led to turn-6 · reviewer · running · Activity ›’;", "the agent shrinks to fit, at most 72px; the badge", "and ‘Activity ›’ never shrink.", "Facts joined by ‘ · ’ at every width, no dot."]),
    ]
    s.notes(40, NY, left)
    s.notes(660, NY, right, "")
    s.save("handoffs.svg")


# ------------------------------------------------------------------ states
def states():
    s = Sheet(1280, 1630, "Activity and Handoffs · every state",
              "Empty, loading, failed, just started, Monaco missing, nothing to say. Flat: no shimmer, no fades; each state says what it is in words.")
    t = s.t
    PW, PH = 380, 300
    cols = [40, 450, 860]

    def panel(i, j, title, handoffs=False, on="Everything"):
        x, y = cols[i], 130 + j * 370
        s.label(x, y - 10, title)
        s.rect(x, y, PW, PH, t["s0"], t["div2"], 2)
        if handoffs:
            return x, y
        f = Feed(s, x, y, PW, phone=True)
        f.oy = y  # the panel's top, for captions
        f.toolbar(on=on)
        return f

    def empty_state(x, y, title, body):
        s.rect(x + PW / 2 - 16, y, 32, 32, t["s2"], r=3)
        s.path(f"M{x + PW / 2 - 7},{y + 12} h14 M{x + PW / 2 - 7},{y + 17} h10 M{x + PW / 2 - 7},{y + 22} h12", t["muted"], 1.3)
        s.text(x + PW / 2, y + 56, title, 14, t["ink"], 600, anchor="middle")
        for k, line in enumerate(body):
            s.text(x + PW / 2, y + 78 + 18 * k, line, 12.5, t["muted"], anchor="middle")

    def sr_only(x, y, words):
        """Text that only screen readers get: drawn as a dashed outline so nobody builds it visible."""
        w = PW - 32
        s.rect(x, y, w, 26, stroke=t["div2"], r=2, dash="4 3")
        s.text(x + 10, y + 17, f"sr-only, not visible: ‘{words}’", 11, t["muted"], 400, True)

    def error(ex, ey, title):
        """The inline load error: title, the server's words, the muted next step, Try again."""
        s.rect(ex, ey, PW - 32, 160, t["s0"], t["div2"], 3)
        s.rect(ex + 14, ey + 16, 16, 16, mix(t["danger"], t["s0"], 0.12), r=8)
        s.text(ex + 22, ey + 28, "!", 12, t["danger"], 700, anchor="middle")
        s.text(ex + 40, ey + 29, title, 13, t["ink"], 600)
        s.text(ex + 40, ey + 50, "GET /api/tasks/task-40: 502 Bad Gateway", 11, t["muted"], 400, True)
        s.text(ex + 40, ey + 72, "If it keeps failing, check that Agentry", 12, t["muted"])
        s.text(ex + 40, ey + 89, "is still running.", 12, t["muted"])
        s.rect(ex + 40, ey + 108, 84, 32, t["s0"], t["div2"], 3)
        s.text(ex + 82, ey + 129, "Try again", 12, t["ink"], 500, anchor="middle")

    # 1. Empty
    f = panel(0, 0, "1 · ACTIVITY, EMPTY (A NEW TASK)")
    empty_state(f.x, f.y + 60, "Nothing here yet", ["Every message, turn and action shows", "here as it happens."])

    # 2. Loading
    f = panel(1, 0, "2 · ACTIVITY, LOADING")
    y = f.y
    for k in range(2):
        s.rect(f.x + 1, y, PW - 2, 44, t["s1"])
        s.line(f.x, y + 44, f.x + PW, y + 44)
        s.rect(f.x + 16, y + 12, 20, 20, t["s3"], r=3)
        s.rect(f.x + 44, y + 16, 110, 12, t["s3"], r=2)
        for r in range(2):
            s.rect(f.x + 44, y + 58 + 22 * r, 220 - 40 * r, 10, t["s2"], r=2)
        y += 44 + 52
    sr_only(f.x + 16, f.oy + PH - 60, "Loading activity…")
    s.text(f.x + 16, f.oy + PH - 16, "aria-busy; static blocks, no shimmer", 11, t["muted"], 400, True)

    # 3. Error
    f = panel(2, 0, "3 · ACTIVITY, FAILED TO LOAD")
    error(f.x + 16, f.y + 20, "Couldn't load the activity")
    s.text(f.x + 16, f.oy + PH - 22, "The toolbar stays. Try again: secondary Button.", 11.5, t["muted"])

    # 4. Running turn with no rows yet
    f = panel(0, 1, "4 · A TURN JUST STARTED, NO ROWS YET")
    top = f.y
    f.band("engineer", status=("running", "info"), time="06:20", dur="8s…", live=True)
    ny = f.y + 24
    s.rect(f.cx, ny - 8, 8, 8, t["ok"], r=4)
    s.text(f.cx + 16, ny, "engineer is starting…", 12.5, t["oki"], 500)
    s.text(f.R + f.tx, ny, "now", 11, t["muted"], 400, True)
    f.y += 40
    f.band("coordinator", status=("ended",), time="06:11", dur="9m")
    my = f.message("coordinator", "engineer", "turn-7")
    f.stamp(my, "06:19")
    f.body([("p", [("Fix the two review notes in ", ""), ("flows.md", "linkm"), ("nt", "nt")])])
    f.rail_line(top, f.y + 10)
    s.text(f.x + 16, f.oy + PH - 58, "The band appears with the turn; the now", 11.5, t["muted"])
    s.text(f.x + 16, f.oy + PH - 40, "row under it says what's happening until", 11.5, t["muted"])
    s.text(f.x + 16, f.oy + PH - 22, "the first row lands. Nothing else waits.", 11.5, t["muted"])

    # 5. Monaco failed
    f = panel(1, 1, "5 · MONACO FAILED TO LOAD")
    f.band("reviewer", status=("running", "info"), time="06:12", dur="4m…", live=True)
    f.action([[("go test ./... -run 'TestLinks'", "")]], ("done",), "06:13", plain=True)
    f.body([("code", [[("linkInto(task, href)", "")]], "ts", True, f.cw)])
    s.text(f.x + 16, f.oy + PH - 40, "Plain mono stays: same text, font, line height.", 11.5, t["muted"])
    s.text(f.x + 16, f.oy + PH - 22, "No error on screen; one console warning.", 11.5, t["muted"])

    # 6. An entry with no text
    f = panel(2, 1, "6 · ENTRIES WITH NO TEXT")
    top = f.y
    f.band("engineer", status=("ended",), time="05:09", dur="48m")
    ry = f.y + 20
    f.icon(ry, "done")
    s.text(f.cx, ry, "Finished, 41k tokens", 12, t["oki"], 600)
    f.stamp(ry, "05:57")
    f.y = ry + 10
    f.band("coordinator", status=("ended",), time="05:00", dur="9m")
    my = f.message("coordinator", "engineer", "turn-5")
    f.stamp(my, "05:08")
    f.y += 8
    f.rail_line(top, f.y)
    s.text(f.x + 16, f.y + 30, "A report or brief with no text shows its", 11.5, t["muted"])
    s.text(f.x + 16, f.y + 48, "line alone: no empty box, no ‘No text’", 11.5, t["muted"])
    s.text(f.x + 16, f.y + 66, "filler, no Show all.", 11.5, t["muted"])

    # 7-9. Handoffs
    x, y = panel(0, 2, "7 · HANDOFFS, EMPTY", True)
    empty_state(x, y + 70, "No handoffs yet", ["When the coordinator hands the task to", "an agent, the brief shows here."])
    x, y = panel(1, 2, "8 · HANDOFFS, LOADING", True)
    s.line(x + 21, y + 20, x + 21, y + 190)
    for k in range(2):
        cy = y + 16 + k * 92
        s.rect(x + 16, cy + 12, 11, 11, t["s3"], r=5.5)
        s.rect(x + 40, cy, PW - 56, 80, t["s0"], t["div"], 3)
        s.rect(x + 52, cy + 12, 150, 12, t["s3"], r=2)
        s.rect(x + 52, cy + 36, 240, 10, t["s2"], r=2)
        s.rect(x + 52, cy + 54, 180, 10, t["s2"], r=2)
    sr_only(x + 16, y + PH - 92, "Loading handoffs…")
    s.text(x + 16, y + PH - 40, "aria-busy; static blocks, no shimmer.", 11.5, t["muted"])
    s.text(x + 16, y + PH - 22, "The words are for screen readers only.", 11.5, t["muted"])
    x, y = panel(2, 2, "9 · HANDOFFS, FAILED TO LOAD", True)
    error(x + 16, y + 24, "Couldn't load the handoffs")
    s.text(x + 16, y + PH - 40, "Commits not loaded or failed: SHAs stay", 11.5, t["muted"])
    s.text(x + 16, y + PH - 22, "plain code. No error: links are extra.", 11.5, t["muted"])

    # 10. A filter that leaves nothing
    f = panel(0, 3, "10 · ACTIVITY, A FILTER WITH NOTHING TO SHOW", on="Actions")
    empty_state(f.x, f.y + 60, "No actions yet", ["Choose Everything to see every row."])
    s.text(f.x + 16, f.oy + PH - 22, "Messages on, none yet: ‘No messages yet’.", 11.5, t["muted"])
    s.save("states.svg")


# ------------------------------------------------------------------ dark
def activity_dark():
    s = Sheet(1280, 960, "Activity · Author bands · dark theme",
              "The desktop fragment on the dark tokens. Chips tint at 22% with the agent colour as the letter; links are info; text stays ink-muted or brighter.", DARK)
    X, Y, W = 40, 120, 820
    s.label(X, Y - 10, "ACTIVITY  ·  dark")
    m = s.mark()
    f, top = desktop_feed(s, X, Y, W, notes=False, length="short")
    f.y += 16
    f.rail_line(top, f.y)
    s.under(m, X, Y, W, f.y - Y)
    s.notes(900, 130, [
        (1, ["Same tokens, dark values: surface-0", "#1a1d23, band surface-1 #16181d,", "hairlines divider #2b2f38."]),
        (2, ["Chips: the agent colour at 22% on", "surface-1, letter in the agent colour:", "4.3:1 or more for every colour."]),
        (3, ["You and Agentry: surface-3 chip,", "ink-secondary letter (6.7:1)."]),
        (4, ["Links: info #38bdf8 (7.9:1), underlined;", "a linked SHA has no code background.", "Muted text #8b919e: 5.3:1 on surface-0.", "Green text stays success #22c55e (7.4:1):", "success-ink is a light-theme fix only."]),
        (5, ["Code: Monaco's vs-dark colours on", "surface-1; plain until on screen."]),
        (6, ["No shadows or glows to separate the", "stuck band: its surface and hairlines do."]),
    ])
    s.save("activity-dark.svg")


activity_desktop()
activity_mobile()
handoffs()
states()
activity_dark()
