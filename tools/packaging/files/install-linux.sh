#!/bin/sh
# Puts Agentry in your applications menu: copies the app and the server
# beside it to ~/.local/share/agentry, the icon, and a menu entry that names
# them by their full paths, so nothing needs to be on your PATH. Run it again
# to update; uninstall.sh, here or copied beside the app, takes it all out.
set -eu
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
data=${XDG_DATA_HOME:-$HOME/.local/share}
case $data in /*) ;; *) data=$HOME/.local/share ;; esac
apps=$data/agentry
icon=$data/icons/hicolor/512x512/apps/agentry.png
entry=$data/applications/agentry.desktop

# put SOURCE TARGET MODE copies SOURCE to a new name beside TARGET, then
# renames it, so updating a program that is running doesn't fail with
# "Text file busy".
put() {
	mkdir -p "$(dirname "$2")"
	cp "$1" "$2.new"
	chmod "$3" "$2.new"
	mv -f "$2.new" "$2"
}

# quoted is $1 as one argument of the entry's Exec: in double quotes, with
# \ " ` $ escaped, then each \ doubled for the string, and % as %% (the
# Desktop Entry spec, Exec key).
quoted() {
	printf '"%s"' "$(printf '%s' "$1" | sed -e 's/[\\"`$]/\\&/g' -e 's/\\/\\\\/g' -e 's/%/%%/g')"
}

put "$here/agentry-desktop" "$apps/agentry-desktop" 755
put "$here/agentry" "$apps/agentry" 755
put "$here/uninstall.sh" "$apps/uninstall.sh" 755
put "$here/agentry.png" "$icon" 644

exec_line="Exec=$(quoted "$apps/agentry-desktop")"
icon_line="Icon=$(printf '%s' "$icon" | sed 's/\\/\\\\/g')"
mkdir -p "$(dirname "$entry")"
while IFS= read -r line; do
	case $line in
	Exec=*) printf '%s\n' "$exec_line" ;;
	Icon=*) printf '%s\n' "$icon_line" ;;
	*) printf '%s\n' "$line" ;;
	esac
done <"$here/agentry.desktop" >"$entry.new"
chmod 644 "$entry.new"
mv -f "$entry.new" "$entry"

# The menu and the icon theme see the new files at once where these exist.
if command -v update-desktop-database >/dev/null; then
	update-desktop-database "$(dirname "$entry")" 2>/dev/null || true
fi
if command -v gtk-update-icon-cache >/dev/null; then
	gtk-update-icon-cache -q -t "$data/icons/hicolor" 2>/dev/null || true
fi

echo "Installed Agentry in your applications menu:"
echo "  $apps/agentry-desktop, with agentry and uninstall.sh beside it"
echo "  $icon"
echo "  $entry"
echo "To uninstall: '$apps/uninstall.sh'"
