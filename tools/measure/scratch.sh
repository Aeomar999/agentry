#!/usr/bin/env bash
# Makes the scratch Python repo the measured task works in (once, before run A).
set -eu; . tools/measure/env.sh
R=$M/scratch-repo
[ -e $R ] && { echo "$R exists"; exit 0; }
mkdir -p $R
cat > $R/durations.py <<'PY'
"""Durations as short strings, like "1h30m"."""

UNITS = {"h": 3600, "m": 60, "s": 1}


def parse_duration(text):
    """Turn "1h30m" into seconds: 5400. "45s" -> 45. Raises ValueError on bad text."""
    total, number = 0, ""
    for ch in text.strip():
        if ch.isdigit():
            number += ch
        elif ch in UNITS and number:
            total += int(number) * UNITS[ch]
            number = ""
        else:
            raise ValueError("bad duration: %r" % text)
    if number:
        raise ValueError("bad duration: %r" % text)
    return total
PY
cat > $R/test_durations.py <<'PY'
import unittest

from durations import parse_duration


class ParseDuration(unittest.TestCase):
    def test_hours_and_minutes(self):
        self.assertEqual(parse_duration("1h30m"), 5400)

    def test_seconds_only(self):
        self.assertEqual(parse_duration("45s"), 45)

    def test_bad_text_raises(self):
        with self.assertRaises(ValueError):
            parse_duration("1x")


if __name__ == "__main__":
    unittest.main()
PY
git -C $R init -q -b main
git -C $R -c user.name=setup -c user.email=setup@example.invalid add -A
git -C $R -c user.name=setup -c user.email=setup@example.invalid commit -qm initial
(cd $R && python3 -m unittest -q) && echo "scratch repo ready: $R"
