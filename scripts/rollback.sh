#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/common.sh"
require_root
require_install
platform_lock
previous="$(cat "$PAGEHUB_ROOT/state/previous-release")"
[[ "$previous" == "$PAGEHUB_ROOT/releases/"* && -x "$previous/bin/pagehub-runner" ]] || die '上一个版本无效'
current="$(readlink -f "$PAGEHUB_ROOT/current")"
if [[ -f "$PAGEHUB_ROOT/config/pagehub.yaml" ]]; then
  "$previous/bin/pagehub-runner" desired validate -c "$PAGEHUB_ROOT/config/pagehub.yaml"
fi
ln -sfnT "$previous" "$PAGEHUB_ROOT/current.rollback"
mv -Tf "$PAGEHUB_ROOT/current.rollback" "$PAGEHUB_ROOT/current"
printf '%s\n' "$current" > "$PAGEHUB_ROOT/state/previous-release"
bash "$previous/scripts/enable-autostart.sh"
systemctl restart pagehub-runner pagehub-manager
if [[ -x "$previous/bin/pagehub-social" ]]; then systemctl restart pagehub-social; else systemctl stop pagehub-social || true; fi
bash "$previous/scripts/start-services.sh"
