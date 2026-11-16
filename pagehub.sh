#!/usr/bin/env bash
set -Eeuo pipefail
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/scripts/lib/common.sh"
require_root
require_install
command="${1:-status}"
[[ $# == 0 ]] || shift
case "$command" in
  status|app|doctor|backup|rollback|remove-app|gateway) runner "$command" "$@" ;;
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
    case "${1:-}" in gateway|portal) platform_compose "$1" logs --tail 100 ;; manager|runner) journalctl -u "pagehub-$1" -n 100 --no-pager ;; *) runner logs "$@" ;; esac
    ;;
  platform-update) bash "$PAGEHUB_ROOT/current/scripts/update.sh" "$@" ;;
  platform-rollback) bash "$PAGEHUB_ROOT/current/scripts/rollback.sh" ;;
  *) die '未知命令，参见 README.md' ;;
esac
