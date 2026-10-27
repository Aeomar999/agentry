#!/bin/sh
# Takes Agentry out of your applications menu: removes what install.sh put
# in, and the copies the README of v0.20.0 had you make by hand, when they
# are Agentry's. Your data, in .agentry in your home folder, stays.
set -eu
# The folder this script is in, when it runs from a file and not a pipe.
here=
[ -f "$0" ] && here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
# The copy install.sh put in uninstalls its own folder: one named agentry,
# holding uninstall.sh and no install.sh. Anywhere else, as the unpacked
# folder or a pipe, it looks in ${XDG_DATA_HOME:-~/.local/share}, so give it
# the XDG_DATA_HOME install.sh had.
if [ -n "$here" ] && [ "${here##*/}" = agentry ] && [ -f "$here/uninstall.sh" ] && [ ! -f "$here/install.sh" ]; then
	apps=$here
	data=$(dirname -- "$apps")
else
	data=${XDG_DATA_HOME:-$HOME/.local/share}
	case $data in /*) ;; *) data=$HOME/.local/share ;; esac
	apps=$data/agentry
fi
bin=$HOME/.local/bin
removed=0

remove() {
	rm -f "$1"
	echo "Removed $1"
	removed=$((removed + 1))
}

# An agentry-desktop is Agentry's when its build info names Agentry's app.
is_agentry_app() {
	[ -f "$1" ] && grep -q -F gitlab.com/dev.chempa/agentry/cmd/agentry-desktop "$1"
}

# ~/.local/bin/agentry-desktop is a hand-made copy of Agentry's when its
# build info names Agentry's desktop app. ~/.local/bin/agentry always stays:
# Agentry's Claude Code and worker installs put the same program there.
if is_agentry_app "$bin/agentry-desktop"; then
	remove "$bin/agentry-desktop"
fi
if [ -e "$bin/agentry" ]; then
	echo "Kept $bin/agentry: Agentry's Claude Code and worker installs use it; delete it only if you copied it there just for the app"
fi

# The menu entry is Agentry's when it is the app's window (StartupWMClass),
# and the icon when that entry was, or it is the same as this release's:
# the one beside this script, or else the one install.sh put in. The README
# had you copy to ~/.local/share, checked first, while that icon is there.
ours=$data/icons/hicolor/512x512/apps/agentry.png
[ -n "$here" ] && [ -f "$here/agentry.png" ] && ours=$here/agentry.png
remove_menu_entry() {
	entry=$1/applications/agentry.desktop
	icon=$1/icons/hicolor/512x512/apps/agentry.png
	if [ -f "$entry" ] && grep -q -x StartupWMClass=Agentry "$entry"; then
		remove "$entry"
		[ -f "$icon" ] && remove "$icon"
	elif [ -f "$icon" ] && cmp -s "$icon" "$ours"; then
		remove "$icon"
	fi
	return 0
}
remove_menu_entry "$HOME/.local/share"
[ "$data" = "$HOME/.local/share" ] || remove_menu_entry "$data"

# The folder is Agentry's install only when its app is Agentry's, so a
# folder taken for it by mistake loses nothing. This script may be the
# installed uninstall.sh, so it goes last.
if is_agentry_app "$apps/agentry-desktop"; then
	for file in agentry-desktop agentry uninstall.sh; do
		[ -f "$apps/$file" ] && remove "$apps/$file"
	done
fi
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
