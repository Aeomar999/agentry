#!/bin/sh
# Starts Agentry and opens it in your browser. Run it again any time to
# open the page; Agentry keeps running when you close the window.
cd "$(dirname "$0")" || exit 1
# Downloaded on a Mac, these files are marked as from the internet; this
# lets the agentry program in this folder run.
[ "$(uname)" = Darwin ] && xattr -dr com.apple.quarantine . 2>/dev/null
./agentry open
