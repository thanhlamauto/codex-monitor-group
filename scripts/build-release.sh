#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
OUTPUT="${1:-$ROOT/dist}"
case "$OUTPUT" in /*) ;; *) OUTPUT="$ROOT/$OUTPUT" ;; esac
mkdir -p "$OUTPUT"

build_agent() {
  os="$1" arch="$2"
  (cd "$ROOT/agent/codex-guard" && CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="-s -w" -o "$OUTPUT/codex-guard-$os-$arch" ./cmd/codex-guard)
}

build_agent linux amd64
build_agent linux arm64
build_agent darwin amd64
build_agent darwin arm64
"$ROOT/scripts/fetch-ccusage.sh" "$OUTPUT"
install -m 0755 "$ROOT/installer/install.sh" "$OUTPUT/install.sh"
install -m 0755 "$ROOT/installer/uninstall.sh" "$OUTPUT/uninstall.sh"
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$OUTPUT" && sha256sum codex-guard-* ccusage-* > checksums.txt)
else
  (cd "$OUTPUT" && shasum -a 256 codex-guard-* ccusage-* > checksums.txt)
fi

echo "Release artifacts written to $OUTPUT"
