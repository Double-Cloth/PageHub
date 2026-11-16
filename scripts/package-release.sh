#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/common.sh"
mode="${1:-source}"
[[ "$mode" == source || "$mode" == offline ]] || die '用法: package-release.sh [source|offline]'
version="$(cat "$PAGEHUB_SOURCE/VERSION")"
arch="$(uname -m)"; [[ "$arch" != x86_64 ]] || arch=amd64; [[ "$arch" != aarch64 ]] || arch=arm64
output="$PAGEHUB_SOURCE/dist"
mkdir -p "$output"
stage="$(mktemp -d)"
trap 'rm -rf -- "$stage"' EXIT
tar -C "$PAGEHUB_SOURCE" --exclude=./.git --exclude=./tmp --exclude=./dist --exclude=./frontend/dist --exclude=./frontend/node_modules --exclude=./bin -cf - . | tar -C "$stage" -xf -
if [[ "$mode" == offline ]]; then
  if [[ -f "$PAGEHUB_ROOT/current/runtime.env" ]]; then source "$PAGEHUB_ROOT/current/runtime.env"; fi
  [[ -x "$PAGEHUB_SOURCE/bin/pagehub-runner" && -f "$PAGEHUB_SOURCE/frontend/dist/index.html" ]] || die '先执行 scripts/build.sh 生成当前架构产物'
  mkdir -p "$stage/bin" "$stage/frontend/dist" "$stage/images"
  cp -a "$PAGEHUB_SOURCE/bin/." "$stage/bin/"
  cp -a "$PAGEHUB_SOURCE/frontend/dist/." "$stage/frontend/dist/"
  python3 "$PAGEHUB_SOURCE/scripts/offline-images.py" "$PAGEHUB_ROOT" "$PAGEHUB_SOURCE" "$stage/images/map.json" > "$stage/images/ids.txt"
  mapfile -t app_images < "$stage/images/ids.txt"
  docker save -o "$stage/images/runtime.tar" "$PAGEHUB_CADDY_IMAGE" "$PAGEHUB_PORTAL_IMAGE" "${app_images[@]}"
  caddy_id="$(docker image inspect "$PAGEHUB_CADDY_IMAGE" --format '{{.Id}}')"
  portal_id="$(docker image inspect "$PAGEHUB_PORTAL_IMAGE" --format '{{.Id}}')"
  printf 'PAGEHUB_OFFLINE=true\nPAGEHUB_CADDY_IMAGE=%s\nPAGEHUB_PORTAL_IMAGE=%s\n' "$caddy_id" "$portal_id" > "$stage/offline.env"
  (cd "$stage" && find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS)
fi
archive="$output/pagehub-$version-$mode-$arch.tar.gz"
tar -C "$stage" -czf "$archive" .
(cd "$output" && sha256sum "$(basename "$archive")" > "$(basename "$archive").sha256")
log "$archive"
