#!/usr/bin/env bash
# Builds CGO-free release archives for every supported platform into dist/.
# Usage: scripts/dist.sh VERSION   (e.g. v0.2.0)
set -euo pipefail

version="${1:?usage: scripts/dist.sh VERSION}"
if [[ ! "$version" =~ ^[A-Za-z0-9._-]+$ ]]; then
  echo "invalid version: $version" >&2
  exit 2
fi

module="github.com/kudaibergenoff/codememory"
out="dist"
targets=(linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64)

rm -rf "$out"
mkdir -p "$out"

# Binary redistribution must carry the licenses (and Apache NOTICE files) of
# every module linked into the binary.
notices="$out/THIRD_PARTY_LICENSES"
go list -deps -f '{{with .Module}}{{if .Version}}{{.Path}} {{.Version}} {{.Dir}}{{end}}{{end}}' ./cmd/codememory |
  sort -u | while read -r mod ver dir; do
    found=0
    for f in "$dir"/LICENSE* "$dir"/LICENCE* "$dir"/COPYING* "$dir"/NOTICE*; do
      [[ -f "$f" ]] || continue
      printf '==== %s %s (%s) ====\n\n' "$mod" "$ver" "$(basename "$f")" >> "$notices"
      cat "$f" >> "$notices"
      printf '\n\n' >> "$notices"
      found=1
    done
    if [[ $found == 0 ]]; then
      echo "no license file for $mod $ver" >&2
      exit 1
    fi
  done
for target in "${targets[@]}"; do
  os="${target%/*}"
  arch="${target#*/}"
  name="codememory_${version}_${os}_${arch}"
  ext=""
  [[ "$os" == windows ]] && ext=".exe"
  mkdir -p "$out/$name"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X $module/internal/cli.Version=$version" \
    -o "$out/$name/codememory$ext" ./cmd/codememory
  cp README.md LICENSE "$notices" "$out/$name/"
  if [[ "$os" == windows ]]; then
    (cd "$out" && zip -qr "$name.zip" "$name")
  else
    tar -C "$out" -czf "$out/$name.tar.gz" "$name"
  fi
  rm -rf "${out:?}/$name"
done

cd "$out"
if command -v sha256sum >/dev/null; then
  sha256sum -- *.tar.gz *.zip > SHA256SUMS
else
  shasum -a 256 -- *.tar.gz *.zip > SHA256SUMS
fi
rm -f THIRD_PARTY_LICENSES
echo "built $(ls -1 | wc -l | tr -d ' ') files in $out/"
