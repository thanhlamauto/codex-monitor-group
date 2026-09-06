#!/bin/sh
set -eu

VERSION="20.0.20"
OUTPUT="${1:-dist}"
mkdir -p "$OUTPUT"

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi
}

fetch_one() {
  platform="$1"
  tar_sha="$2"
  case "$platform" in
    linux-amd64) package_platform="linux-x64" ;;
    linux-arm64) package_platform="linux-arm64" ;;
    darwin-amd64) package_platform="darwin-x64" ;;
    darwin-arm64) package_platform="darwin-arm64" ;;
    *) echo "unsupported platform: $platform" >&2; exit 1 ;;
  esac
  url="https://registry.npmjs.org/@ccusage/ccusage-${package_platform}/-/ccusage-${package_platform}-${VERSION}.tgz"
  archive="$(mktemp)"
  extract="$(mktemp -d)"
  trap 'rm -f "$archive"; rm -rf "$extract"' EXIT INT TERM
  curl -fL --proto '=https' --tlsv1.2 "$url" -o "$archive"
  actual="$(sha256_file "$archive")"
  if [ "$actual" != "$tar_sha" ]; then
    echo "ccusage package checksum mismatch for $platform" >&2
    exit 1
  fi
  tar -xzf "$archive" -C "$extract"
  install -m 0755 "$extract/package/bin/ccusage" "$OUTPUT/ccusage-${platform}"
  rm -f "$archive"
  rm -rf "$extract"
  trap - EXIT INT TERM
}

fetch_one linux-amd64 "819aca18837f85a596c330ac8c8dbeee750ae44af330475ae82b74b8b23b6874"
fetch_one linux-arm64 "f7f9e5ba90f15bfd1db020e5a76660c3d91e67e1ffc0ec6801eaa900622be6b7"
fetch_one darwin-amd64 "be8e268364c5d4a7d695c3c4e906e27bc433d8c7b0bda742396975a9ccb5a5fd"
fetch_one darwin-arm64 "ad27629a45e0a3e45eb167080db0aad7545080134da6849b6ea774b3ed6899e1"
