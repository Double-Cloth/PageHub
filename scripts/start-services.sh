#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/common.sh"
require_root
docker info >/dev/null
docker network inspect pagehub-ingress >/dev/null
docker image inspect "$(platform_compose gateway config --images)" "$(platform_compose portal config --images)" >/dev/null
platform_compose portal up -d --no-build --pull never
platform_compose gateway up -d --no-build --pull never
failed=0
systemctl start pagehub-runner.service pagehub-manager.service pagehub-social.service || failed=1
runner recover "${1:-}" || failed=1
bash "$PAGEHUB_ROOT/current/scripts/healthcheck.sh" || failed=1
exit "$failed"
