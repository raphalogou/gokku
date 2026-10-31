#!/bin/sh
# Remove Gokku's service, sudoers rule and binary. The data directory (/var/lib/gokku,
# which holds the database and the secret key) and /etc/gokku are kept unless --purge is
# given, which also removes the gokku user. Run as root.
set -eu

PURGE=no
case "${1:-}" in
--purge) PURGE=yes ;;
"") ;;
*)
	printf 'usage: uninstall.sh [--purge]\n' >&2
	exit 2
	;;
esac

[ "$(id -u)" -eq 0 ] || {
	printf 'gokku uninstall: run as root\n' >&2
	exit 1
}

say() { printf '\033[1m-----> %s\033[0m\n' "$*"; }

if systemctl list-unit-files gokku.service >/dev/null 2>&1; then
	say "Stopping gokku"
	systemctl disable --now gokku >/dev/null 2>&1 || true
fi
rm -f /etc/systemd/system/gokku.service /etc/sudoers.d/gokku /usr/local/bin/gokku
systemctl daemon-reload

if [ "$PURGE" = yes ]; then
	say "Removing /var/lib/gokku, /etc/gokku and the gokku user"
	rm -rf /var/lib/gokku /etc/gokku
	if id gokku >/dev/null 2>&1; then
		userdel gokku
	fi
	say "Gokku is removed"
else
	say "Gokku is removed. Kept /var/lib/gokku (database and secret key) and /etc/gokku; use --purge to delete them."
fi
