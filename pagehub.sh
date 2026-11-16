#!/usr/bin/env bash
set -Eeuo pipefail
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/scripts/lib/common.sh"
require_root
require_install
command="${1:-status}"
[[ $# == 0 ]] || shift
case "$command" in
  admin)
    [[ "${1:-}" == set-password ]] || die '用法: pagehub.sh admin set-password'
    read -r -s -p '设置管理员密码（至少 12 字节）: ' password; printf '\n'
    read -r -s -p '再次输入密码: ' repeated; printf '\n'
    [[ "$password" == "$repeated" ]] || die '两次密码不一致'
    printf '%s\n' "$password" | "$PAGEHUB_ROOT/current/bin/pagehub-manager" set-password
    chown root:pagehub "$PAGEHUB_ROOT/secrets/admin-password.json"
    systemctl restart pagehub-manager
    ;;
  offline)
    case "${1:-}" in
      fingerprint) "$PAGEHUB_ROOT/current/bin/pagehub-manager" offline-fingerprint "${2:?需要应用 ID}" ;;
      approve) "$PAGEHUB_ROOT/current/bin/pagehub-manager" approve-offline "${2:?需要应用 ID}" "${3:?需要验收记录路径}"; runner gateway reload ;;
      *) die '用法: pagehub.sh offline [fingerprint <app-id>|approve <app-id> <记录.json>]' ;;
    esac
    ;;
  backup|restore)
    if [[ "${1:-}" == portal ]]; then
      shift
      bash "$PAGEHUB_ROOT/current/scripts/portal-state.sh" "$command" "$@"
    else
      runner "$command" "$@"
    fi
    ;;
  status|compose-review|app|doctor|rollback|remove-app|gateway) runner "$command" "$@" ;;
  add-app|update|adopt-app)
    [[ $# -gt 0 ]] || die '需要应用包路径，update 还需应用 ID'
    dry=false; approved=false
    for arg in "$@"; do [[ "$arg" != --dry-run ]] || dry=true; [[ "$arg" != --approved-sha256* ]] || approved=true; done
    if [[ "$dry" == true || "$approved" == true ]]; then runner "$command" "$@"; exit; fi
    preview="$(runner "$command" "$@" --dry-run)"
    printf '%s\n' "$preview"
    [[ -t 0 ]] || die '非交互导入需要审核来源并传入 --approved-sha256 <精确包摘要>'
    read -r -p '已审核应用来源、代码、权限和持久化影响？输入 TRUST 确认: ' answer
    [[ "$answer" == TRUST ]] || die '未确认导入'
    digest="$(printf '%s' "$preview" | python3 -c 'import json,sys; print(json.load(sys.stdin)["sha256"])')"
    runner "$command" "$@" --approved-sha256 "$digest"
    ;;
  logs)
    case "${1:-}" in gateway|portal) platform_compose "$1" logs --tail 100 ;; manager|runner|social) journalctl -u "pagehub-$1" -n 100 --no-pager ;; *) runner logs "$@" ;; esac
    ;;
  platform-update) bash "$PAGEHUB_ROOT/current/scripts/update.sh" "$@" ;;
  platform-rollback) bash "$PAGEHUB_ROOT/current/scripts/rollback.sh" ;;
  *) die '未知命令，参见 README.md' ;;
esac
