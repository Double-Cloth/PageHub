#!/usr/bin/env bash
set -Eeuo pipefail
[[ "${PAGEHUB_ROOT:-}" == *pagehub-test* ]] || { echo '测试根目录必须包含 pagehub-test'; exit 1; }
source /workspace/scripts/lib/common.sh
[[ ! -e "$PAGEHUB_ROOT/config/apps.yaml" ]] || die '测试根目录必须为空'
docker network inspect pagehub-ingress >/dev/null 2>&1 && die '发现现有 PageHub 网络，请使用独立 Docker Engine 测试'
for project in pagehub-gateway pagehub-portal pagehub-static-demo pagehub-image-demo pagehub-compose-demo pagehub-private-demo pagehub-preserve-demo pagehub-adopt-demo pagehub-volume-demo pagehub-bundled-demo pagehub-unhealthy-demo pagehub-test-legacy; do
  [[ -z "$(docker ps -aq --filter "label=com.docker.compose.project=$project")" ]] || die "发现已有工程 $project，请使用空的隔离 Docker Engine"
done
mkdir -p "$PAGEHUB_ROOT/"{config,state,gateway/public,apps,secrets,releases/test}
tar -C /workspace --exclude=./.git --exclude=./frontend/node_modules --exclude=./dist --exclude=./tmp -cf - . | tar -C "$PAGEHUB_ROOT/releases/test" -xf -
ln -s "$PAGEHUB_ROOT/releases/test" "$PAGEHUB_ROOT/current"
chmod +x "$PAGEHUB_ROOT/current/bin/"*
printf '{"schema_version":1,"apps":[]}\n' > "$PAGEHUB_ROOT/config/apps.yaml"
printf 'test-only-admin-token\n' > "$PAGEHUB_ROOT/secrets/admin.token"
printf 'PAGEHUB_ROOT=%s\nPAGEHUB_GATEWAY_PORT=%s\nPAGEHUB_CADDY_IMAGE=%s\nPAGEHUB_PORTAL_IMAGE=%s\nPAGEHUB_OFFLINE=false\n' "$PAGEHUB_ROOT" "$PAGEHUB_GATEWAY_PORT" "$PAGEHUB_CADDY_IMAGE" "$PAGEHUB_PORTAL_IMAGE" > "$PAGEHUB_ROOT/config/pagehub.env"
printf '{"version":"test"}\n' > "$PAGEHUB_ROOT/state/install-state.json"
mkdir -p /run/pagehub /run/pagehub-public
docker network create pagehub-ingress
runner gateway reload
docker build -t "$PAGEHUB_PORTAL_IMAGE" -f /workspace/infrastructure/portal/Dockerfile /workspace
platform_compose portal up -d --pull never --no-build
platform_compose gateway up -d --pull never --no-build
"$PAGEHUB_ROOT/current/bin/pagehub-runner" serve &
runner_pid=$!
"$PAGEHUB_ROOT/current/bin/pagehub-manager" &
manager_pid=$!
cleanup() {
  kill "$runner_pid" "$manager_pid" 2>/dev/null || true
  for id in static-demo image-demo compose-demo private-demo preserve-demo adopt-demo volume-demo bundled-demo; do runner remove-app "$id" 2>/dev/null || true; done
  if [[ -f /tmp/legacy-compose.json ]]; then docker compose -p pagehub-test-legacy -f /tmp/legacy-compose.json down || true; fi
  platform_compose gateway down || true
  platform_compose portal down || true
  docker network rm pagehub-ingress || true
}
trap cleanup EXIT
for attempt in {1..30}; do curl -fsS "http://127.0.0.1:$PAGEHUB_GATEWAY_PORT/healthz" >/dev/null && break; sleep 1; done
mkdir -p /tmp/pagehub-packages
python3 - /workspace /tmp/pagehub-packages <<'PY'
import pathlib, sys, zipfile
source, dest = map(pathlib.Path, sys.argv[1:])
for name in ['static-site', 'image-web', 'compose-web']:
    with zipfile.ZipFile(dest / (name + '.zip'), 'w', zipfile.ZIP_DEFLATED) as archive:
        for file in (source / 'apps/examples' / name).rglob('*'):
            if file.is_file():
                archive.write(file, file.relative_to(source / 'apps/examples' / name))
PY
for name in static-site image-web compose-web; do
  archive="/tmp/pagehub-packages/$name.zip"
  before="$(sha256sum "$PAGEHUB_ROOT/config/apps.yaml")"
  runner add-app "$archive" --dry-run
  [[ "$before" == "$(sha256sum "$PAGEHUB_ROOT/config/apps.yaml")" ]]
  runner add-app "$archive" --approved-sha256 "$(sha256sum "$archive" | cut -d ' ' -f1)"
done
python3 /workspace/tests/integration/verify.py
kill "$runner_pid" "$manager_pid"
sleep 1
curl -fsS "http://127.0.0.1:$PAGEHUB_GATEWAY_PORT/" >/dev/null
curl -fsS "http://127.0.0.1:$PAGEHUB_GATEWAY_PORT/static-demo/" >/dev/null
curl -fsS "http://127.0.0.1:$PAGEHUB_GATEWAY_PORT/compose-demo/api/value" | grep -q persistent-test-value
curl -fsS "http://127.0.0.1:$PAGEHUB_GATEWAY_PORT/_pagehub/catalog.json" | grep -q static-demo
"$PAGEHUB_ROOT/current/bin/pagehub-runner" serve &
runner_pid=$!
"$PAGEHUB_ROOT/current/bin/pagehub-manager" &
manager_pid=$!
log '控制平面退出后主页与已运行应用仍可访问'
log '真实容器集成验收通过'
if [[ "${PAGEHUB_TEST_KEEP_RUNNING:-false}" == true ]]; then
  touch "$PAGEHUB_ROOT/state/acceptance-passed"
  trap 'exit 0' TERM INT
  while true; do sleep 30 & wait $!; done
fi
