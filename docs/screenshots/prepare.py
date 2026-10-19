#!/usr/bin/env python3
"""Prepare the README screenshots from full-screen originals.

Usage: python3 docs/screenshots/prepare.py SOURCE_DIR OUTPUT_DIR

SOURCE_DIR holds the originals, named as in SHOTS below and taken at
SOURCE_SIZE. Each one is cropped to the page, has its personal details
covered with flat boxes, is scaled to WIDTH and saved as an optimised PNG
of the same name in OUTPUT_DIR. Needs Pillow.
"""

import sys
from pathlib import Path

from PIL import Image

WIDTH = 1600

# The originals' size. Every box below is measured in these pixels.
SOURCE_SIZE = (2877, 1800)

# Everything above the app's header: the desktop bar and the browser's title bar.
PAGE = (0, 91, 2877, 1800)

PANEL = (32, 35, 40)
CHROME = (28, 30, 35)
TERMINAL = (22, 23, 27)

# The home path in the status bar, on every page.
STATUS_PATH = ((376, 1764, 590, 1794), CHROME)

# Boxes are (left, top, right, bottom) in the original's pixels, each with
# the colour around it so the cover reads as empty space.
SHOTS = {
    "tasks-board.png": [STATUS_PATH],
    "task-page.png": [
        STATUS_PATH,
        ((1264, 240, 1490, 268), PANEL),  # repo path under the title
        ((1422, 690, 1893, 717), PANEL),  # worktree path in the first handoff
        ((1092, 1300, 1558, 1327), PANEL),  # worktree path in the third
        ((1340, 1571, 1880, 1598), PANEL),  # machine name and path in the fourth
    ],
    "agents-org-chart.png": [STATUS_PATH],
    "agents-teams-claude.png": [
        STATUS_PATH,
        # The machines each agent runs on, one line per card.
        ((418, 634, 800, 658), PANEL),
        ((908, 634, 1290, 658), PANEL),
        ((1398, 634, 1780, 658), PANEL),
        ((1888, 634, 2270, 658), PANEL),
        ((2378, 634, 2760, 658), PANEL),
        # Claude Code's "Transcript saving is off" warning.
        ((390, 1654, 2200, 1684), TERMINAL),
    ],
}


def prepare(source, covers):
    image = Image.open(source).convert("RGB")
    if image.size != SOURCE_SIZE:
        sys.exit(
            f"{source} is {image.width}x{image.height}, not "
            f"{SOURCE_SIZE[0]}x{SOURCE_SIZE[1]}: the cover boxes would miss"
        )
    for box, colour in covers:
        image.paste(colour, box)
    image = image.crop(PAGE)
    height = round(image.height * WIDTH / image.width)
    return image.resize((WIDTH, height), Image.LANCZOS)


def main(source_dir, output_dir):
    output_dir.mkdir(parents=True, exist_ok=True)
    for name, covers in SHOTS.items():
        image = prepare(source_dir / name, covers)
        image.save(output_dir / name, optimize=True)


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    main(Path(sys.argv[1]), Path(sys.argv[2]))
