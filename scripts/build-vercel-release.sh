#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
PUBLIC="$ROOT/public"
DOWNLOADS="$PUBLIC/downloads"

mkdir -p "$PUBLIC"
"$ROOT/scripts/build-release.sh" "$DOWNLOADS"
install -m 0755 "$ROOT/installer/install.sh" "$PUBLIC/install.sh"

echo "Vercel static release artifacts written to $PUBLIC"
