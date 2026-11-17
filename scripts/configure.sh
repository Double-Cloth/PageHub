#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/common.sh"
require_root
getent group pagehub >/dev/null || groupadd --system pagehub
id pagehub >/dev/null 2>&1 || useradd --system --gid pagehub --home-dir "$PAGEHUB_ROOT" --shell /usr/sbin/nologin pagehub
id pagehub-social >/dev/null 2>&1 || useradd --system --gid pagehub --home-dir "$PAGEHUB_ROOT/state/social" --shell /usr/sbin/nologin pagehub-social
chown root:pagehub "$PAGEHUB_ROOT"
chmod 0750 "$PAGEHUB_ROOT"
for directory in releases config gateway state apps; do install -d -m 0750 -o root -g pagehub "$PAGEHUB_ROOT/$directory"; done
chmod 2750 "$PAGEHUB_ROOT/config"
install -d -m 0710 -o root -g pagehub "$PAGEHUB_ROOT/secrets"
install -d -m 0700 "$PAGEHUB_ROOT/backups"
install -d -m 0750 -o pagehub -g pagehub "$PAGEHUB_ROOT/state/manager" "$PAGEHUB_ROOT/state/portal"
install -d -m 0750 -o root -g pagehub "$PAGEHUB_ROOT/state/offline"
install -d -m 0700 -o pagehub-social -g pagehub "$PAGEHUB_ROOT/state/social"
if [[ ! -f "$PAGEHUB_ROOT/config/pagehub.env" ]]; then
  origin="${PAGEHUB_PUBLIC_ORIGIN:-http://localhost:$PAGEHUB_GATEWAY_PORT}"
  [[ "$origin" =~ ^https?://[a-zA-Z0-9.:-]+$ ]] || die 'PUBLIC_ORIGIN 应为完整 HTTP(S) Origin'
  [[ "$PAGEHUB_GATEWAY_PORT" =~ ^[0-9]+$ && "$PAGEHUB_GATEWAY_PORT" -ge 1024 && "$PAGEHUB_GATEWAY_PORT" -le 65535 ]] || die '网关端口必须为 1024..65535'
  printf 'PAGEHUB_ROOT=%s\nPAGEHUB_PUBLIC_ORIGIN=%s\nPAGEHUB_GATEWAY_PORT=%s\nPAGEHUB_FRP_MODE=external\nPAGEHUB_ADMIN_ACCESS=local\nPAGEHUB_DEV_IMAGES=%s\nPAGEHUB_IMPORT_BUNDLED=%s\nPAGEHUB_OFFLINE=%s\nPAGEHUB_CADDY_IMAGE=%s\nPAGEHUB_PORTAL_IMAGE=%s\n' "$PAGEHUB_ROOT" "$origin" "$PAGEHUB_GATEWAY_PORT" "$PAGEHUB_DEV_IMAGES" "${PAGEHUB_IMPORT_BUNDLED:-true}" "$PAGEHUB_OFFLINE" "$PAGEHUB_CADDY_IMAGE" "$PAGEHUB_PORTAL_IMAGE" > "$PAGEHUB_ROOT/config/pagehub.env"
  chmod 600 "$PAGEHUB_ROOT/config/pagehub.env"
  printf 'PAGEHUB_ADMIN_PORT=%s\n' "$PAGEHUB_ADMIN_PORT" >> "$PAGEHUB_ROOT/config/pagehub.env"
  if [[ -n "${PAGEHUB_CONFIG_FILE:-}" ]]; then printf 'PAGEHUB_CONFIG_FILE=%s\n' "$PAGEHUB_CONFIG_FILE" >> "$PAGEHUB_ROOT/config/pagehub.env"; fi
fi
if [[ ! -f "$PAGEHUB_ROOT/secrets/admin.token" ]]; then
  python3 -c 'import secrets; print(secrets.token_urlsafe(48))' > "$PAGEHUB_ROOT/secrets/admin.token"
  chown root:pagehub "$PAGEHUB_ROOT/secrets/admin.token"
  chmod 640 "$PAGEHUB_ROOT/secrets/admin.token"
fi
chown root:pagehub "$PAGEHUB_ROOT/secrets/admin.token"
chmod 640 "$PAGEHUB_ROOT/secrets/admin.token"
[[ -f "$PAGEHUB_ROOT/config/apps.yaml" ]] || printf '{"schema_version":1,"apps":[]}\n' > "$PAGEHUB_ROOT/config/apps.yaml"
chown root:pagehub "$PAGEHUB_ROOT/config/apps.yaml"
chmod 640 "$PAGEHUB_ROOT/config/apps.yaml"
