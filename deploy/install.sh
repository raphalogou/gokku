#!/bin/sh
# Install or upgrade Gokku on a Dokku server (DESIGN.md, section 8). Run as root:
#
#   curl -fsSL https://github.com/<owner>/gokku/releases/latest/download/install.sh | sudo sh
#
# Environment:
#   GOKKU_VERSION   release tag to install, such as v1.2.0 (default: the latest release)
#   GOKKU_REPO      GitHub repository to download from, owner/name (default below)
#   GOKKU_DOWNLOAD_URL  base URL holding gokku_linux_<arch> and checksums.txt, for a mirror
#                   (default: the GitHub release of GOKKU_VERSION)
#   GOKKU_BINARY    install this local binary instead of downloading one
#
# The script is idempotent: running it again upgrades the binary and restarts the service,
# and keeps /etc/gokku/gokku.env and the data in /var/lib/gokku. Migrations run on start.
set -eu

REPO="${GOKKU_REPO:-}" # TODO: set the default to the published repository, owner/name
VERSION="${GOKKU_VERSION:-latest}"
BIN=/usr/local/bin/gokku
UNIT=/etc/systemd/system/gokku.service
SUDOERS=/etc/sudoers.d/gokku
ENV_DIR=/etc/gokku
ENV_FILE="$ENV_DIR/gokku.env"
DATA_DIR=/var/lib/gokku
USER_NAME=gokku

say() { printf '\033[1m-----> %s\033[0m\n' "$*"; }
fail() {
	printf 'gokku install: %s\n' "$*" >&2
	exit 1
}
need() { command -v "$1" >/dev/null 2>&1 || fail "$1 is required but was not found."; }

[ "$(id -u)" -eq 0 ] || fail "run as root, for example: curl -fsSL <url> | sudo sh"
need systemctl
need visudo
need sha256sum
need install

DOKKU="$(command -v dokku || true)"
[ -n "$DOKKU" ] || fail "dokku was not found. Install Dokku first: https://dokku.com/docs/getting-started/installation/"

case "$(uname -m)" in
x86_64 | amd64) ARCH=amd64 ;;
aarch64 | arm64) ARCH=arm64 ;;
*) fail "unsupported architecture $(uname -m); build from source instead." ;;
esac

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM

# 1. Get the binary and check it.
if [ -n "${GOKKU_BINARY:-}" ]; then
	say "Using the local binary $GOKKU_BINARY"
	cp "$GOKKU_BINARY" "$TMP/gokku"
else
	if [ -n "${GOKKU_DOWNLOAD_URL:-}" ]; then
		BASE="${GOKKU_DOWNLOAD_URL%/}"
	elif [ -z "$REPO" ]; then
		fail "set GOKKU_REPO=owner/name, GOKKU_DOWNLOAD_URL, or GOKKU_BINARY to a local binary."
	elif [ "$VERSION" = latest ]; then
		BASE="https://github.com/$REPO/releases/latest/download"
	else
		BASE="https://github.com/$REPO/releases/download/$VERSION"
	fi
	if command -v curl >/dev/null 2>&1; then
		fetch() { curl -fsSL --retry 3 -o "$2" "$1"; }
	elif command -v wget >/dev/null 2>&1; then
		fetch() { wget -q -O "$2" "$1"; }
	else
		fail "curl or wget is required to download Gokku."
	fi
	ASSET="gokku_linux_$ARCH"
	say "Downloading $ASSET ($VERSION)"
	fetch "$BASE/$ASSET" "$TMP/$ASSET" || fail "download failed: $BASE/$ASSET"
	fetch "$BASE/checksums.txt" "$TMP/checksums.txt" || fail "download failed: $BASE/checksums.txt"
	(cd "$TMP" && grep " $ASSET\$" checksums.txt | sha256sum -c --status) ||
		fail "the checksum of $ASSET does not match checksums.txt; nothing was installed."
	mv "$TMP/$ASSET" "$TMP/gokku"
fi
chmod 0755 "$TMP/gokku"
NEW_VERSION="$("$TMP/gokku" version)" || fail "the binary does not run on this system."

# 2. The service user. Its home is the data directory; it cannot log in.
if ! id "$USER_NAME" >/dev/null 2>&1; then
	say "Creating the $USER_NAME user"
	useradd --system --user-group --home-dir "$DATA_DIR" --no-create-home \
		--shell /usr/sbin/nologin --comment "Gokku" "$USER_NAME"
fi

# 3. The binary.
UPGRADE=no
[ -x "$BIN" ] && UPGRADE=yes
say "Installing $BIN ($NEW_VERSION)"
install -m 0755 -o root -g root "$TMP/gokku" "$BIN.new"
mv -f "$BIN.new" "$BIN"

# 4. Configuration, kept on upgrade. Readable by the service only.
install -d -m 0750 -o root -g "$USER_NAME" "$ENV_DIR"
if [ ! -f "$ENV_FILE" ]; then
	say "Writing $ENV_FILE"
	cat >"$ENV_FILE" <<EOF
# Gokku configuration. Restart after a change: systemctl restart gokku
# All settings: gokku serve --help
GOKKU_LISTEN=127.0.0.1:8080
GOKKU_DATA_DIR=$DATA_DIR
GOKKU_DOKKU_BIN=$DOKKU
# Host shown in "git remote add" instructions; the browser's host when unset.
#GOKKU_DOKKU_HOST=dokku.example.com
# Set when a reverse proxy on this host forwards to Gokku (README, "Expose it").
#GOKKU_TRUSTED_PROXIES=127.0.0.1/32
EOF
	chown root:"$USER_NAME" "$ENV_FILE"
	chmod 0640 "$ENV_FILE"
fi

# 5. The sudoers rule, validated before it is moved into place (SEC-7). It lets the
# service user run any dokku command: Gokku's roles are the policy.
say "Writing $SUDOERS"
# No PAM session per call: Gokku runs dokku every few seconds, and sudo still logs each command.
printf '# Written by the Gokku installer. Gokku runs dokku through sudo -n.\nDefaults:%s !pam_session\n%s ALL=(root) NOPASSWD: %s\n' \
	"$USER_NAME" "$USER_NAME" "$DOKKU" >"$TMP/sudoers"
visudo -cf "$TMP/sudoers" >/dev/null || fail "the generated sudoers file is invalid; nothing was changed."
install -m 0440 -o root -g root "$TMP/sudoers" "$SUDOERS"

# 6. The systemd unit.
say "Writing $UNIT"
cat >"$TMP/gokku.service" <<'EOF'
[Unit]
Description=Gokku
Documentation=https://dokku.com
After=network-online.target docker.service
Wants=network-online.target

[Service]
User=gokku
Group=gokku
ExecStart=/usr/local/bin/gokku serve
EnvironmentFile=-/etc/gokku/gokku.env
StateDirectory=gokku
StateDirectoryMode=0700
Restart=on-failure
RestartSec=2
# Do not add NoNewPrivileges, ProtectHome or ProtectSystem=strict. sudo needs setuid,
# and Dokku writes outside the service's own directories.

[Install]
WantedBy=multi-user.target
EOF
install -m 0644 -o root -g root "$TMP/gokku.service" "$UNIT"
systemctl daemon-reload

# 7. Check that the service user can reach Dokku, then start.
if ! su -s /bin/sh "$USER_NAME" -c "sudo -n '$DOKKU' version" >/dev/null 2>&1; then
	printf 'gokku install: warning: "sudo -n %s version" fails for the %s user; Gokku will report Dokku as unreachable.\n' \
		"$DOKKU" "$USER_NAME" >&2
fi
if [ "$UPGRADE" = yes ]; then
	say "Restarting gokku"
	systemctl enable gokku >/dev/null 2>&1
	systemctl restart gokku
else
	say "Starting gokku"
	systemctl enable --now gokku >/dev/null 2>&1
fi

# 8. Wait until it answers.
LISTEN="$(sed -n 's/^GOKKU_LISTEN=//p' "$ENV_FILE" | tail -n 1)"
LISTEN="${LISTEN:-127.0.0.1:8080}"
healthy() {
	systemctl is-active --quiet gokku || return 1
	if command -v curl >/dev/null 2>&1; then
		curl -fs -o /dev/null "http://$LISTEN/healthz"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O /dev/null "http://$LISTEN/healthz"
	fi
}
i=0
until healthy; do
	i=$((i + 1))
	if [ "$i" -ge 30 ]; then
		fail "gokku did not start; see: journalctl -u gokku -n 50"
	fi
	sleep 1
done

say "Gokku $NEW_VERSION is running on http://$LISTEN"
if [ "$UPGRADE" = no ]; then
	cat <<EOF

Next steps:
  1. Open Gokku. From your computer, through an SSH tunnel:
       ssh -L 8080:$LISTEN <user>@<this server>
     then browse to http://localhost:8080 and create the first admin.
  2. To give it a domain and TLS, follow "Expose it through the nginx that Dokku uses"
     in the README (deploy/nginx.conf.example), or docs/caddy.md for Caddy.

Configuration: $ENV_FILE    Data: $DATA_DIR    Logs: journalctl -u gokku
EOF
fi
