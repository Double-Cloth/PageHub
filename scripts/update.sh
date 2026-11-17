#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/common.sh"
require_root
require_install
platform_lock
[[ $# == 1 && -d "$1" ]] || die '用法: pagehub.sh platform-update <已解压版本目录>'
export PAGEHUB_SOURCE="$(cd -- "$1" && pwd)"
[[ -f "$PAGEHUB_SOURCE/VERSION" && -f "$PAGEHUB_SOURCE/scripts/deploy.sh" ]] || die '版本目录无效'
snapshot="$PAGEHUB_ROOT/backups/platform-$(date -u +%Y%m%dT%H%M%S)"
mkdir -m 700 "$snapshot"
cp -a "$PAGEHUB_ROOT/config" "$PAGEHUB_ROOT/secrets" "$snapshot/"
old="$(readlink -f "$PAGEHUB_ROOT/current")"
if [[ -f "$PAGEHUB_ROOT/state/previous-release" ]]; then cp -a "$PAGEHUB_ROOT/state/previous-release" "$snapshot/previous-release"; fi
bash "$PAGEHUB_SOURCE/scripts/build.sh"
if [[ -f "$PAGEHUB_ROOT/config/pagehub.yaml" ]]; then
  "$PAGEHUB_SOURCE/bin/pagehub-runner" desired validate -c "$PAGEHUB_ROOT/config/pagehub.yaml"
fi
recover_old() {
  log '升级失败，恢复旧平台版本'
  ln -sfnT "$old" "$PAGEHUB_ROOT/current.rollback"
  mv -Tf "$PAGEHUB_ROOT/current.rollback" "$PAGEHUB_ROOT/current"
  cp -a "$snapshot/config/." "$PAGEHUB_ROOT/config/"
  cp -a "$snapshot/secrets/." "$PAGEHUB_ROOT/secrets/"
  if [[ -f "$snapshot/previous-release" ]]; then cp -a "$snapshot/previous-release" "$PAGEHUB_ROOT/state/previous-release"; else rm -f "$PAGEHUB_ROOT/state/previous-release"; fi
  bash "$old/scripts/enable-autostart.sh"
  systemctl restart pagehub-runner pagehub-manager
  if [[ -x "$old/bin/pagehub-social" ]]; then systemctl restart pagehub-social; else systemctl stop pagehub-social || true; fi
  bash "$old/scripts/start-services.sh" || true
  exit 1
}
trap recover_old ERR
bash "$PAGEHUB_SOURCE/scripts/configure.sh"
bash "$PAGEHUB_SOURCE/scripts/deploy.sh"
bash "$PAGEHUB_ROOT/current/scripts/enable-autostart.sh"
systemctl restart pagehub-runner pagehub-manager pagehub-social
bash "$PAGEHUB_ROOT/current/scripts/start-services.sh"
bash "$PAGEHUB_ROOT/current/scripts/healthcheck.sh"
date -u +%FT%TZ > "$PAGEHUB_ROOT/current/.pagehub-verified"
trap - ERR
if [[ -f "$PAGEHUB_ROOT/config/pagehub.yaml" ]]; then
  retain="$(runner desired retention)"
  if ! python3 "$PAGEHUB_ROOT/current/scripts/prune-platform.py" "$PAGEHUB_ROOT" "$retain"; then log '历史版本清理失败，请检查权限与 previous-release 引用'; fi
fi
log '平台更新完成，应用数据与独立版本保持在稳定目录'
