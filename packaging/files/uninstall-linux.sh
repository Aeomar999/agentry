#!/bin/sh
# Takes Agentry out of your applications menu: removes what install.sh put
# in, and the copies the README of v0.20.0 had you make by hand, when they
# are Agentry's. Your data, in .agentry in your home folder, stays.
set -eu
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
# Run from the unpacked folder (install.sh beside it), it looks in
# ${XDG_DATA_HOME:-~/.local/share}, so give it the XDG_DATA_HOME install.sh
# had. The copy install.sh put in uninstalls its own folder.
if [ -f "$here/install.sh" ]; then
	data=${XDG_DATA_HOME:-$HOME/.local/share}
	case $data in /*) ;; *) data=$HOME/.local/share ;; esac
	apps=$data/agentry
else
	apps=$here
	data=$(dirname -- "$apps")
fi
bin=$HOME/.local/bin
removed=0

remove() {
	rm -f "$1"
	echo "Removed $1"
	removed=$((removed + 1))
}

# ~/.local/bin/agentry-desktop is a hand-made copy of Agentry's when its
# build info names Agentry's desktop app. ~/.local/bin/agentry always stays:
# Agentry's Claude Code and worker installs put the same program there.
if [ -f "$bin/agentry-desktop" ] && grep -q -F gitlab.com/dev.chempa/agentry/cmd/agentry-desktop "$bin/agentry-desktop"; then
	remove "$bin/agentry-desktop"
fi
if [ -e "$bin/agentry" ]; then
	echo "Kept $bin/agentry: Agentry's Claude Code and worker installs use it; delete it only if you copied it there just for the app"
fi

# The menu entry is Agentry's when it is the app's window (StartupWMClass),
# and the icon when that entry was, or it is the same as this release's:
# the one beside this script, or else the one install.sh put in. The README
# had you copy to ~/.local/share, checked first, while that icon is there.
ours=$here/agentry.png
[ -f "$ours" ] || ours=$data/icons/hicolor/512x512/apps/agentry.png
for share in "$HOME/.local/share" "$data"; do
	entry=$share/applications/agentry.desktop
	icon=$share/icons/hicolor/512x512/apps/agentry.png
	if [ -f "$entry" ] && grep -q -x StartupWMClass=Agentry "$entry"; then
		remove "$entry"
		[ -f "$icon" ] && remove "$icon"
	elif [ -f "$icon" ] && cmp -s "$icon" "$ours"; then
		remove "$icon"
	fi
done

# This script may be the installed uninstall.sh, so it goes last.
for file in agentry-desktop agentry uninstall.sh; do
	[ -f "$apps/$file" ] && remove "$apps/$file"
done
[ -d "$apps" ] && rmdir "$apps" 2>/dev/null && echo "Removed $apps"

# The menu and the icon theme forget them at once where these exist.
if command -v update-desktop-database >/dev/null; then
	update-desktop-database "$data/applications" 2>/dev/null || true
fi
if command -v gtk-update-icon-cache >/dev/null; then
	gtk-update-icon-cache -q -t "$data/icons/hicolor" 2>/dev/null || true
fi

if [ "$removed" = 0 ]; then
	echo "Agentry was not installed; nothing to remove."
fi
echo "Your data in $HOME/.agentry stays; delete it yourself to start afresh."
