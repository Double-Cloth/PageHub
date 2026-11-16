#!/usr/bin/env bash
set -Eeuo pipefail
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/scripts/lib/common.sh"
require_root
require_install
platform_lock
case "${1:-}" in
  --check) bash "$PAGEHUB_ROOT/current/scripts/healthcheck.sh" ;;
  ''|--boot) bash "$PAGEHUB_ROOT/current/scripts/start-services.sh" "${1:-}" ;;
  --force)
    log '强制恢复会停止常驻应用，请先完成应用定义的写入停止窗口'
    [[ -t 0 ]] || die '--force 需要交互终端确认'
    read -r -p '输入 FORCE 确认已完成安全停止准备: ' answer
    [[ "$answer" == FORCE ]] || die '未确认'
    bash "$PAGEHUB_ROOT/current/scripts/start-services.sh" --force
    ;;
  *) die '用法: restart.sh [--check|--boot|--force]' ;;
esac
