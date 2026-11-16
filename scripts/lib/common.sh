#!/usr/bin/env bash
set -Eeuo pipefail
umask 027

PAGEHUB_SOURCE="${PAGEHUB_SOURCE:-$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)}"
PAGEHUB_ROOT="${PAGEHUB_ROOT:-/srv/pagehub}"
[[ "$PAGEHUB_ROOT" =~ ^/[a-zA-Z0-9/_-]+$ && "$PAGEHUB_ROOT" != / ]] || { echo 'PAGEHUB_ROOT 必须是安全的 Linux 绝对目录' >&2; exit 1; }
if [[ -f "$PAGEHUB_ROOT/config/pagehub.env" ]]; then
  set -a
  source "$PAGEHUB_ROOT/config/pagehub.env"
  set +a
fi
export PAGEHUB_ROOT PAGEHUB_SOURCE
export PAGEHUB_GATEWAY_PORT="${PAGEHUB_GATEWAY_PORT:-8080}"
export PAGEHUB_OFFLINE="${PAGEHUB_OFFLINE:-false}"
export PAGEHUB_DEV_IMAGES="${PAGEHUB_DEV_IMAGES:-false}"
export PAGEHUB_CADDY_IMAGE="${PAGEHUB_CADDY_IMAGE:-caddy:2.10.2-alpine}"
export PAGEHUB_PORTAL_IMAGE="${PAGEHUB_PORTAL_IMAGE:-pagehub-portal:0.1.0}"

log() { printf '[PageHub] %s\n' "$*"; }
die() { printf '[PageHub] %s\n' "$*" >&2; exit 1; }
require_root() { [[ $EUID == 0 ]] || die '请使用 sudo bash 执行此命令'; }
require_install() { [[ -f "$PAGEHUB_ROOT/state/install-state.json" ]] || die '安装状态缺失，请先执行 install.sh'; }
platform_lock() { mkdir -p "$PAGEHUB_ROOT/state"; exec 9>"$PAGEHUB_ROOT/state/platform.lock"; flock -n 9 || die '已有安装或恢复任务运行中'; }
runner() (
  if [[ -f "$PAGEHUB_ROOT/current/runtime.env" ]]; then set -a; source "$PAGEHUB_ROOT/current/runtime.env"; set +a; fi
  "$PAGEHUB_ROOT/current/bin/pagehub-runner" "$@"
)
platform_compose() (
  if [[ -f "$PAGEHUB_ROOT/current/runtime.env" ]]; then set -a; source "$PAGEHUB_ROOT/current/runtime.env"; set +a; fi
  docker compose --project-name "pagehub-$1" --env-file "$PAGEHUB_ROOT/config/pagehub.env" -f "$PAGEHUB_ROOT/current/infrastructure/$1/compose.yaml" "${@:2}"
)
trap 'printf "[PageHub] 执行失败，行号 %s。请运行 pagehub.sh doctor 查看状态。\n" "$LINENO" >&2' ERR
