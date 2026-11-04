#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")/.."

version="0.76.0"
goos="${1:-}"
goarch="${2:-}"
if [[ -z "$goos" || -z "$goarch" ]]; then
  echo "usage: $0 <darwin|windows> <amd64|arm64|universal>" >&2
  exit 2
fi

if [[ "$goos" == "darwin" && "$goarch" == "universal" ]]; then
  "$0" darwin arm64
  "$0" darwin amd64
  exit 0
fi

case "$goos/$goarch" in
  darwin/arm64)
    asset="oxfmt-aarch64-apple-darwin.tar.gz"
    member="oxfmt-aarch64-apple-darwin"
    sha256="2ad708e9afe27c0b4430d27fbd008d815bb3c4479eaacfabb4f3d4b1b1c2ae9e"
    ;;
  darwin/amd64)
    asset="oxfmt-x86_64-apple-darwin.tar.gz"
    member="oxfmt-x86_64-apple-darwin"
    sha256="c8c35ae8245108b1dace679bd516745727456385230dd899ba3a2aa3595681f7"
    ;;
  windows/arm64)
    asset="oxfmt-aarch64-pc-windows-msvc.zip"
    member="oxfmt-aarch64-pc-windows-msvc.exe"
    sha256="7f51f757d661f74699b03c691ee85815620398a9223df2a7fc3cc074f4c6545c"
    ;;
  windows/amd64)
    asset="oxfmt-x86_64-pc-windows-msvc.zip"
    member="oxfmt-x86_64-pc-windows-msvc.exe"
    sha256="3982af5a54069dc6a9ccb1fce2153d2b03caab8cd79feaecc5e93f02ec83b851"
    ;;
  *)
    echo "unsupported Oxfmt platform: $goos/$goarch" >&2
    exit 2
    ;;
esac

cache_root="${WXAPKG_CACHE_DIR:-$HOME/.local/share/codex-tools/cache/wxapkg/oxfmt}"
archive="$cache_root/v$version/$asset"
mkdir -p "$(dirname "$archive")"

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

download() {
  if [[ -f "$archive" ]] && [[ "$(hash_file "$archive")" == "$sha256" ]]; then
    return
  fi
  url="https://github.com/oxc-project/oxc/releases/download/oxfmt_v$version/$asset"
  proxy="${WXAPKG_PROXY:-http://127.0.0.1:7890}"
  common=(
    --fail
    --location
    --retry 3
    --connect-timeout 15
    --max-time 180
    --speed-limit 20480
    --speed-time 20
    --continue-at -
    --output "$archive"
  )
  if [[ "${WXAPKG_USE_PROXY:-0}" == "1" ]]; then
    curl --proxy "$proxy" "${common[@]}" "$url"
    return
  fi
  if curl "${common[@]}" "$url"; then
    return
  fi
  echo "Direct download was slow or unavailable; retrying via $proxy" >&2
  curl --proxy "$proxy" "${common[@]}" "$url"
}

download
actual="$(hash_file "$archive")"
if [[ "$actual" != "$sha256" ]]; then
  echo "Oxfmt checksum mismatch: got $actual, want $sha256" >&2
  exit 1
fi

destination="resources/$goos-$goarch"
mkdir -p "$destination"
binary="$destination/oxfmt"
if [[ "$goos" == "windows" ]]; then
  binary="$binary.exe"
  unzip -p "$archive" "$member" > "$binary"
else
  tar -xOf "$archive" "$member" > "$binary"
fi
chmod 0755 "$binary"
echo "Prepared Oxfmt v$version: $binary"
