"""Agentry's figures: the svg-diagrams kit's layout and labelling, drawn flat
to match the app: square-ish corners, hairline borders, no shadows, no side
stripes. Colour is a role, as in the kit: clay the coordinator, blue you and
your tools, green agents and good outcomes, amber decisions and waiting on
you, red refusal, teal storage."""
import html, math

FONT = "Inter, ui-sans-serif, -apple-system, 'Segoe UI', Helvetica, Arial, sans-serif"
MONO = "ui-monospace, 'SF Mono', Menlo, Consolas, monospace"
PAPER, PANEL, CARD, LINE = "#fafaf8", "#f2f2ef", "#ffffff", "#dddcd6"
INK, MUTED, FAINT = "#1c1c1b", "#62615c", "#93918a"
COLORS = {"clay": "#d4673f", "blue": "#3f7cc0", "green": "#4f8a3c", "amber": "#b7840d",
          "red": "#c0442f", "teal": "#2f8f8f", "muted": FAINT}

def esc(t): return html.escape(str(t), quote=True)
def tw(text, size, weight=400): return len(text) * size * (0.58 if weight >= 600 else 0.54)

class Figure:
    def __init__(self, width, height, title, kicker, subtitle):
        self.w, self.h, self.title, self.kicker, self.subtitle, self.parts = width, height, title, kicker, subtitle, []
    def add(self, p): self.parts.append(p)
    def svg(self):
        markers = "".join(
            f'<marker id="a-{n}" viewBox="0 0 10 10" refX="8.5" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">'
            f'<path d="M1,1.5 L8.5,5 L1,8.5" fill="none" stroke="{c}" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></marker>'
            for n, c in COLORS.items())
        return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {self.w} {self.h}" width="{self.w}" height="{self.h}" font-family="{FONT}" role="img" aria-label="{esc(self.title)}">'
                f'<title>{esc(self.title)}</title><defs>{markers}</defs>'
                f'<rect width="{self.w}" height="{self.h}" rx="8" fill="{PAPER}"/>'
                f'<rect x="0.75" y="0.75" width="{self.w-1.5}" height="{self.h-1.5}" rx="7.5" fill="none" stroke="{LINE}" stroke-width="1.5"/>'
                f'<text x="48" y="54" font-size="11.5" font-weight="700" letter-spacing="1.6" fill="{COLORS["clay"]}">{esc(self.kicker.upper())}</text>'
                f'<text x="48" y="86" font-size="27" font-weight="700" fill="{INK}" letter-spacing="-0.4">{esc(self.title)}</text>'
                f'<text x="48" y="114" font-size="15" fill="{MUTED}">{esc(self.subtitle)}</text>'
                + "".join(self.parts) + "</svg>\n")

    def panel(self, x, y, w, h, label):
        self.add(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="6" fill="{PANEL}" stroke="{LINE}" stroke-width="1.2"/>')
        self.add(f'<text x="{x+20}" y="{y+28}" font-size="11" font-weight="700" letter-spacing="1.4" fill="{FAINT}">{esc(label.upper())}</text>')

    def card(self, x, y, w, h, title, sub="", color="muted", kicker="", mono=False, tag=""):
        c = COLORS[color]
        self.add(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="4" fill="{CARD}" stroke="{c}" stroke-opacity="0.55" stroke-width="1.2"/>')
        lines = [l for l in ([sub] if isinstance(sub, str) else list(sub)) if l]
        block = (17 if kicker else 0) + 20 + 18 * len(lines)
        top, left = y + (h - block) / 2, x + 18
        if kicker:
            self.add(f'<text x="{left}" y="{top+11}" font-size="10.5" font-weight="700" letter-spacing="1.3" fill="{c}">{esc(kicker.upper())}</text>')
            top += 17
        fam = f' font-family="{MONO}"' if mono else ""
        self.add(f'<text x="{left}" y="{top+15}" font-size="15.5" font-weight="650"{fam} fill="{INK}">{esc(title)}</text>')
        for i, l in enumerate(lines):
            self.add(f'<text x="{left}" y="{top+35+18*i}" font-size="12.5" fill="{MUTED}">{esc(l)}</text>')
        if tag:
            w2 = tw(tag, 10.5, 700) + 14
            self.add(f'<rect x="{x+w-w2-10}" y="{y+10}" width="{w2}" height="20" rx="3" fill="{c}" fill-opacity="0.12"/>')
            self.add(f'<text x="{x+w-w2/2-10}" y="{y+24}" font-size="10.5" font-weight="700" text-anchor="middle" fill="{c}">{esc(tag)}</text>')

    def store(self, cx, y, w, h, title, sub, mono_lines=()):
        t = COLORS["teal"]; rx, ry = w / 2, 14; l, r = cx - rx, cx + rx
        self.add(f'<path d="M{l},{y+ry} V{y+h-ry} A{rx},{ry} 0 0 0 {r},{y+h-ry} V{y+ry}" fill="{CARD}" stroke="{t}" stroke-width="1.4"/>')
        self.add(f'<ellipse cx="{cx}" cy="{y+ry}" rx="{rx}" ry="{ry}" fill="#e4f1f1" stroke="{t}" stroke-width="1.4"/>')
        self.add(f'<text x="{cx}" y="{y+58}" font-size="16" font-weight="700" text-anchor="middle" fill="{INK}">{esc(title)}</text>')
        self.add(f'<text x="{cx}" y="{y+77}" font-size="12.5" text-anchor="middle" fill="{MUTED}">{esc(sub)}</text>')
        for i, l in enumerate(mono_lines):
            self.add(f'<text x="{cx}" y="{y+98+16*i}" font-size="11" font-family="{MONO}" text-anchor="middle" fill="{t}">{esc(l)}</text>')

    def hub(self, cx, cy, r, title, sub=()):
        self.add(f'<circle cx="{cx}" cy="{cy}" r="{r}" fill="{COLORS["clay"]}"/>')
        self.add(f'<text x="{cx}" y="{cy-4}" font-size="18" font-weight="700" text-anchor="middle" fill="#ffffff">{esc(title)}</text>')
        for i, l in enumerate(sub):
            self.add(f'<text x="{cx}" y="{cy+18+16*i}" font-size="12.5" text-anchor="middle" fill="#fff1ea">{esc(l)}</text>')

    def number(self, cx, cy, n, color="clay"):
        self.add(f'<rect x="{cx-13}" y="{cy-13}" width="26" height="26" rx="4" fill="{COLORS[color]}"/>')
        self.add(f'<text x="{cx}" y="{cy+4.5}" font-size="13" font-weight="700" text-anchor="middle" fill="#ffffff">{esc(n)}</text>')

    def chips(self, y, steps, gap=20):
        widths = [tw(t, 12.5, 600) + 26 for t, _ in steps]
        x = (self.w - sum(widths) - gap * (len(steps) - 1)) / 2
        for i, ((t, col), w) in enumerate(zip(steps, widths)):
            c = COLORS[col]
            self.add(f'<rect x="{x}" y="{y-15}" width="{w}" height="30" rx="4" fill="{c}" fill-opacity="0.10" stroke="{c}" stroke-opacity="0.5"/>')
            self.add(f'<text x="{x+w/2}" y="{y+4.5}" font-size="12.5" font-weight="650" text-anchor="middle" fill="{c}">{esc(t)}</text>')
            if i < len(steps) - 1:
                self.arrow(f"M{x+w+3},{y} H{x+w+gap-3}", "muted", width=1.5)
            x += w + gap

    def rule(self, y, label="", note=""):
        self.add(f'<line x1="48" y1="{y}" x2="{self.w-48}" y2="{y}" stroke="{LINE}"/>')
        if label: self.note(48, y + 30, label.upper(), size=11, color=FAINT, weight=700)
        if note: self.note(48 + len(label) * 8.7 + 16, y + 30, note)

    def arrow(self, d, color="muted", dashed=False, both=False, width=1.8, head=True):
        dash = ' stroke-dasharray="5 5"' if dashed else ""
        s = f' marker-start="url(#a-{color})"' if both else ""
        e = f' marker-end="url(#a-{color})"' if head else ""
        self.add(f'<path d="{d}" fill="none" stroke="{COLORS[color]}" stroke-width="{width}" stroke-linecap="round" stroke-linejoin="round"{dash}{s}{e}/>')

    def pill(self, cx, cy, text, color="muted", mono=False):
        w = tw(text, 11.5, 600) + 18
        fam = f' font-family="{MONO}"' if mono else ""
        self.add(f'<rect x="{cx-w/2}" y="{cy-11}" width="{w}" height="22" rx="4" fill="{CARD}" stroke="{COLORS[color]}" stroke-opacity="0.5"/>')
        self.add(f'<text x="{cx}" y="{cy+4}" font-size="11.5" font-weight="600" text-anchor="middle"{fam} fill="{COLORS[color]}">{esc(text)}</text>')

    def note(self, x, y, text, size=12.5, color=None, anchor="start", weight=400, mono=False):
        fam = f' font-family="{MONO}"' if mono else ""
        self.add(f'<text x="{x}" y="{y}" font-size="{size}" font-weight="{weight}" text-anchor="{anchor}"{fam} fill="{color or MUTED}">{esc(text)}</text>')

    def spokes(self, hx, hy, hr, cx, cy, cw, ch, out_label="brief", back_label="report", out="clay", back="green"):
        dx, dy = cx - hx, cy - hy; L = math.hypot(dx, dy); ux, uy = dx / L, dy / L; px, py = -uy * 11, ux * 11
        st = (hx + ux * (hr + 12), hy + uy * (hr + 12)); en = edge_point(cx, cy, cw, ch, hx, hy, 10)
        self.arrow(f"M{st[0]+px:.1f},{st[1]+py:.1f} L{en[0]+px:.1f},{en[1]+py:.1f}", out)
        self.arrow(f"M{en[0]-px:.1f},{en[1]-py:.1f} L{st[0]-px:.1f},{st[1]-py:.1f}", back)
        mx, my = (st[0] + en[0]) / 2, (st[1] + en[1]) / 2
        self.pill(mx + px * 2.6, my + py * 2.6, out_label, out)
        self.pill(mx - px * 2.6, my - py * 2.6, back_label, back)

def edge_point(cx, cy, w, h, tx, ty, pad=6):
    dx, dy = tx - cx, ty - cy
    s = min((w / 2 + pad) / abs(dx) if dx else math.inf, (h / 2 + pad) / abs(dy) if dy else math.inf)
    return cx + dx * s, cy + dy * s
