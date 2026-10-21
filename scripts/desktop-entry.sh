#!/usr/bin/env sh
# Writes io.github.omamessenger.desktop, so OmaMessenger appears in
# Omarchy's apps menu (SUPER+ALT+SPACE). Sourced by install-local.sh and
# install-helper.sh, the two explicit user install actions. Not meant to
# run on its own.
#
#   . "$dir/scripts/desktop-entry.sh"
#   install_desktop_entry

# install_desktop_entry writes the user-scoped .desktop entry atomically
# (temp file, then mv within the same directory). It never runs
# update-desktop-database: that rewrites the whole shared
# mimeinfo.cache in this directory, every other app's entry included,
# and this entry declares no MimeType for it to index in the first
# place.
install_desktop_entry() {
    desktop_dir="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
    desktop_file="$desktop_dir/io.github.omamessenger.desktop"
    mkdir -p "$desktop_dir"

    desktop_tmp=$(mktemp "$desktop_dir/.io.github.omamessenger.desktop.XXXXXX")
    cat > "$desktop_tmp" <<'EOF'
[Desktop Entry]
Type=Application
Name=OmaMessenger
Comment=Chat with WhatsApp and Telegram from Omarchy
Exec=omarchy-shell shell summon io.github.omamessenger "{}"
Icon=internet-chat
Categories=Network;InstantMessaging;
Terminal=false
StartupNotify=false
EOF
    chmod 644 "$desktop_tmp"
    mv -f "$desktop_tmp" "$desktop_file"
}
