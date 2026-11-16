#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/common.sh"
require_root
require_install
platform_lock
case "${1:-}" in
  backup)
    target="$PAGEHUB_ROOT/backups/portal-$(date -u +%Y%m%dT%H%M%S)-$$"
    mkdir -m 0700 "$target"
    "$PAGEHUB_ROOT/current/bin/pagehub-social" backup "$target/messages.db"
    for directory in portal offline; do
      [[ ! -d "$PAGEHUB_ROOT/state/$directory" ]] || cp -a "$PAGEHUB_ROOT/state/$directory" "$target/$directory"
    done
    (cd "$target" && find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS)
    log "$target"
    ;;
  restore)
    [[ $# == 2 && -d "$2" ]] || die '用法: portal-state.sh restore <备份目录>'
    snapshot="$(realpath -- "$2")"
    [[ -f "$snapshot/SHA256SUMS" && -f "$snapshot/messages.db" ]] || die '备份缺少数据库或校验清单'
    (cd "$snapshot" && sha256sum --check SHA256SUMS)
    systemctl stop pagehub-social pagehub-manager
    trap 'systemctl start pagehub-social pagehub-manager' EXIT
    # 先保留当前数据库，再恢复经过校验的快照。
    safety="$PAGEHUB_ROOT/backups/portal-before-restore-$(date -u +%Y%m%dT%H%M%S)-$$"
    mkdir -m 0700 "$safety"
    "$PAGEHUB_ROOT/current/bin/pagehub-social" backup "$safety/messages.db"
    for directory in portal offline; do
      [[ ! -d "$PAGEHUB_ROOT/state/$directory" ]] || cp -a "$PAGEHUB_ROOT/state/$directory" "$safety/$directory"
    done
    install -m 0600 -o pagehub-social -g pagehub "$snapshot/messages.db" "$PAGEHUB_ROOT/state/social/messages.db.restore"
    rm -f -- "$PAGEHUB_ROOT/state/social/messages.db-wal" "$PAGEHUB_ROOT/state/social/messages.db-shm"
    mv -f -- "$PAGEHUB_ROOT/state/social/messages.db.restore" "$PAGEHUB_ROOT/state/social/messages.db"
    if [[ -f "$snapshot/portal/ui-settings.json" ]]; then
      install -m 0600 -o pagehub -g pagehub "$snapshot/portal/ui-settings.json" "$PAGEHUB_ROOT/state/portal/ui-settings.json"
    fi
    log '弹幕与展示配置已恢复，离线审核记录须与当前登记版本重新核对'
    ;;
  *) die '用法: portal-state.sh [backup|restore <备份目录>]' ;;
esac
