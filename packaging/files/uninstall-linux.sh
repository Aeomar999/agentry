#!/bin/sh
# Takes Agentry out of your applications menu: removes what install.sh put
# in, and the copies the README of v0.20.0 had you make by hand, when they
# are Agentry's. Your data, in .agentry in your home folder, stays.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
data=${XDG_DATA_HOME:-$HOME/.local/share}
case $data in /*) ;; *) data=$HOME/.local/share ;; esac
apps=$data/agentry
bin=$HOME/.local/bin
removed=0

remove() {
	rm -f "$1"
	echo "Removed $1"
	removed=$((removed + 1))
}

# The hand-made copies are Agentry's when: ~/.local/bin/agentry-desktop
# names Agentry's desktop app in its build info; ~/.local/bin/agentry is the
# same, byte for byte, as the agentry here or installed, and was copied with
# that agentry-desktop. Agentry's worker and Claude Code installs put an
# agentry there too, so any other one stays.
if [ -f "$bin/agentry-desktop" ] && grep -q -F gitlab.com/dev.chempa/agentry/cmd/agentry-desktop "$bin/agentry-desktop"; then
	remove "$bin/agentry-desktop"
	if [ -f "$bin/agentry" ] && { cmp -s "$bin/agentry" "$here/agentry" || cmp -s "$bin/agentry" "$apps/agentry"; }; then
		remove "$bin/agentry"
	elif [ -e "$bin/agentry" ]; then
		echo "Kept $bin/agentry: it is not this release's; delete it yourself if you copied it there for the app"
	fi
fi

# The menu entry is Agentry's when it is the app's window (StartupWMClass),
# and the icon when that entry was, or it is this release's. install.sh
# writes to $data; the README had you copy to ~/.local/share.
for share in "$data" "$HOME/.local/share"; do
	entry=$share/applications/agentry.desktop
	icon=$share/icons/hicolor/512x512/apps/agentry.png
	if [ -f "$entry" ] && grep -q -x StartupWMClass=Agentry "$entry"; then
		remove "$entry"
		[ -f "$icon" ] && remove "$icon"
	elif [ -f "$icon" ] && cmp -s "$icon" "$here/agentry.png"; then
		remove "$icon"
	fi
done

for program in agentry-desktop agentry; do
	[ -f "$apps/$program" ] && remove "$apps/$program"
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
