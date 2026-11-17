#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/common.sh"
require_root
version="$(cat "$PAGEHUB_SOURCE/VERSION")"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die 'VERSION 格式无效'
release="$PAGEHUB_ROOT/releases/$version-$(date -u +%Y%m%dT%H%M%S)-$$"
mkdir -p "$release"
tar -C "$PAGEHUB_SOURCE" --exclude=./.git --exclude=./tmp --exclude=./frontend/node_modules --exclude=./dist --exclude=./images -cf - . | tar -C "$release" -xf -
python3 "$PAGEHUB_SOURCE/scripts/source-version.py" "$PAGEHUB_SOURCE" "$release/source-version.json"
[[ -f "$release/frontend/dist/index.html" ]] || { mkdir -p "$release/frontend/dist"; cp -a "$PAGEHUB_SOURCE/frontend/dist/." "$release/frontend/dist/"; }
chmod +x "$release/bin/"* "$release/"*.sh "$release/scripts/"*.sh
chown -R root:pagehub "$release"
chmod -R g-w,o-rwx "$release"
caddy_id="$(docker image inspect "$PAGEHUB_CADDY_IMAGE" --format '{{.Id}}')"
portal_id="$(docker image inspect "$PAGEHUB_PORTAL_IMAGE" --format '{{.Id}}')"
printf 'PAGEHUB_CADDY_IMAGE=%s\nPAGEHUB_PORTAL_IMAGE=%s\n' "$caddy_id" "$portal_id" > "$release/runtime.env"
chmod 0640 "$release/runtime.env"
chown root:pagehub "$release/runtime.env"
docker network inspect pagehub-ingress >/dev/null 2>&1 || docker network create pagehub-ingress >/dev/null
install -d -m 0755 "$PAGEHUB_ROOT/gateway/public" /run/pagehub-public
install -d -m 0755 -o pagehub-social -g pagehub /run/pagehub-social
"$release/bin/pagehub-runner" gateway reload
for component in gateway portal; do
  docker compose --project-name "pagehub-$component" --env-file "$PAGEHUB_ROOT/config/pagehub.env" -f "$release/infrastructure/$component/compose.yaml" config -q
done
if [[ -L "$PAGEHUB_ROOT/current" ]]; then readlink "$PAGEHUB_ROOT/current" > "$PAGEHUB_ROOT/state/previous-release"; fi
ln -sfnT "$release" "$PAGEHUB_ROOT/current.pending"
mv -Tf "$PAGEHUB_ROOT/current.pending" "$PAGEHUB_ROOT/current"
