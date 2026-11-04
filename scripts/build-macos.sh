#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This build needs macOS: code signing and the disk image use macOS tools." >&2
  exit 1
fi

./scripts/fetch-oxfmt.sh darwin universal
go tool mygo build -platform darwin/universal

build_dir="build/darwin-universal"
app="$(find "$build_dir" -maxdepth 1 -type d -name '*.app' -print -quit)"
dmg="$(find "$build_dir" -maxdepth 1 -type f -name '*.dmg' -print -quit)"
if [[ -z "$app" || -z "$dmg" ]]; then
  echo "The MyGo build produced no app or disk image in $build_dir." >&2
  exit 1
fi

bundle_id="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$app/Contents/Info.plist")"
name="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleDisplayName' "$app/Contents/Info.plist")"
requirement="designated => identifier \"$bundle_id\""

# Ad-hoc signatures normally designate one exact code hash. Every rebuild
# then invalidates the app's TCC grant, which looks like an enabled Full Disk
# Access switch that still denies WeChat's container. An identifier-based
# requirement keeps the grant valid across unsigned builds.
codesign --force --deep --sign - \
  --identifier "$bundle_id" \
  --preserve-metadata=entitlements \
  -r="$requirement" \
  "$app"

codesign --verify --deep --strict "$app"
if ! codesign -d -r- "$app" 2>&1 | grep -Fq "$requirement"; then
  echo "The app's stable designated requirement was not applied." >&2
  exit 1
fi

stage="$(mktemp -d)"
trap 'rm -r "$stage"' EXIT
ditto "$app" "$stage/$(basename "$app")"
ln -s /Applications "$stage/Applications"
hdiutil create \
  -volname "$name" \
  -srcfolder "$stage" \
  -ov \
  -format UDZO \
  "$dmg" >/dev/null
codesign --force --sign - "$dmg"

echo "Stable Full Disk Access identity: $requirement"
echo "Built $app"
echo "Built $dmg"
