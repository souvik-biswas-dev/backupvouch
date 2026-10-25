#!/usr/bin/env bash
# Fetch IBM Plex Sans and IBM Plex Mono (SIL Open Font License 1.1) as woff2 files from the
# @fontsource npm packages into internal/dashboard/web/fonts, so the dashboard needs no CDN.
# Run it once with `make dashboard-fonts` and commit the files. Needs node and npm.
# PLEX_SANS_VERSION and PLEX_MONO_VERSION pin the packages (default: the latest 5.x).
set -euo pipefail

cd "$(dirname "$0")/.."
dest=internal/dashboard/web/fonts
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

npm install --prefix "$tmp" --no-audit --no-fund --ignore-scripts \
  "@fontsource/ibm-plex-sans@${PLEX_SANS_VERSION:-5}" \
  "@fontsource/ibm-plex-mono@${PLEX_MONO_VERSION:-5}"

sans="$tmp/node_modules/@fontsource/ibm-plex-sans"
mono="$tmp/node_modules/@fontsource/ibm-plex-mono"

mkdir -p "$dest"
for f in ibm-plex-sans-latin-400-normal ibm-plex-sans-latin-600-normal; do
  cp "$sans/files/$f.woff2" "$dest/"
done
cp "$mono/files/ibm-plex-mono-latin-400-normal.woff2" "$dest/"

{
  echo "IBM Plex Sans"
  cat "$sans/LICENSE"
  echo
  echo "IBM Plex Mono"
  cat "$mono/LICENSE"
} > "$dest/LICENSE-IBM-Plex.txt"

{
  echo "@fontsource/ibm-plex-sans $(node -p "require('$sans/package.json').version")"
  echo "@fontsource/ibm-plex-mono $(node -p "require('$mono/package.json').version")"
} > "$dest/VERSIONS.txt"

echo "Fonts written to $dest:"
ls -l "$dest"
