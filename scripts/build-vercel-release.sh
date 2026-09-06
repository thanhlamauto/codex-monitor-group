#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
PUBLIC="$ROOT/public"
DOWNLOADS="$PUBLIC/downloads"

mkdir -p "$PUBLIC"
"$ROOT/scripts/build-release.sh" "$DOWNLOADS"
install -m 0755 "$ROOT/installer/install.sh" "$PUBLIC/install.sh"
install -m 0644 "$ROOT/installer/install.ps1" "$PUBLIC/install.ps1"

echo "Vercel static release artifacts written to $PUBLIC"
