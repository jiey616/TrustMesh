#!/bin/sh
set -eu

STATE_DIR="${DATA_DIR:-/var/lib/clawsynapse}"
TRANSFER_DIR="${TRANSFER_DIR:-/var/lib/trustmesh-transfers}"

mkdir -p "$STATE_DIR" "$TRANSFER_DIR"

chown -R clawsynapse:clawsynapse "$STATE_DIR" "$TRANSFER_DIR"
chmod 755 "$TRANSFER_DIR"

exec su-exec clawsynapse /usr/local/bin/clawsynapsed "$@"
