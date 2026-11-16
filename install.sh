#!/usr/bin/env bash
set -Eeuo pipefail
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/scripts/lib/common.sh"
require_root
platform_lock
if [[ -f "$PAGEHUB_ROOT/state/install-state.json" ]]; then
  log '检测到已安装平台，保留配置与凭据并执行恢复'
  bash "$PAGEHUB_ROOT/current/scripts/start-services.sh"
  exit 0
fi
bash "$PAGEHUB_SOURCE/scripts/preflight.sh"
bash "$PAGEHUB_SOURCE/scripts/configure.sh"
source "$PAGEHUB_ROOT/config/pagehub.env"
bash "$PAGEHUB_SOURCE/scripts/build.sh"
bash "$PAGEHUB_SOURCE/scripts/deploy.sh"
bash "$PAGEHUB_ROOT/current/scripts/enable-autostart.sh"
bash "$PAGEHUB_ROOT/current/scripts/start-services.sh"
bash "$PAGEHUB_ROOT/current/scripts/import-bundled-apps.sh"
bash "$PAGEHUB_ROOT/current/scripts/healthcheck.sh"
printf '{"version":"%s","installed_at":"%s","root":"%s"}\n' "$(cat "$PAGEHUB_ROOT/current/VERSION")" "$(date -u +%FT%TZ)" "$PAGEHUB_ROOT" > "$PAGEHUB_ROOT/state/install-state.json.pending"
mv -f "$PAGEHUB_ROOT/state/install-state.json.pending" "$PAGEHUB_ROOT/state/install-state.json"
flock -u 9
systemctl start pagehub.service
log '安装完成，管理员 Token 保存在 secrets/admin.token，请通过 SSH 转发 8091 端口访问 /admin/'
