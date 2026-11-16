#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/common.sh"
if [[ "$PAGEHUB_OFFLINE" == true ]]; then
  [[ -x "$PAGEHUB_SOURCE/bin/pagehub-runner" && -x "$PAGEHUB_SOURCE/bin/pagehub-manager" && -x "$PAGEHUB_SOURCE/bin/pagehub-social" && -f "$PAGEHUB_SOURCE/frontend/dist/index.html" ]] || die '离线包缺少目标架构二进制或前端产物'
  [[ -f "$PAGEHUB_SOURCE/images/runtime.tar" ]] || die '离线包缺少 images/runtime.tar'
  (cd "$PAGEHUB_SOURCE" && sha256sum --check SHA256SUMS)
  docker load -i "$PAGEHUB_SOURCE/images/runtime.tar"
  install -m 0600 "$PAGEHUB_SOURCE/images/map.json" "$PAGEHUB_ROOT/config/offline-images.json"
else
  mkdir -p "$PAGEHUB_SOURCE/bin"
  docker run --rm -v "$PAGEHUB_SOURCE:/src" -w /src/backend -e CGO_ENABLED=0 golang:1.25.4-alpine sh -c 'go mod download && go build -trimpath -o /src/bin/pagehub-runner ./cmd/runner && go build -trimpath -o /src/bin/pagehub-manager ./cmd/manager && go build -trimpath -o /src/bin/pagehub-social ./cmd/social'
  docker run --rm -v "$PAGEHUB_SOURCE/frontend:/src" -w /src node:22.16.0-alpine sh -c 'npm ci && npm run build'
  docker pull "$PAGEHUB_CADDY_IMAGE"
  docker build -t "$PAGEHUB_PORTAL_IMAGE" -f "$PAGEHUB_SOURCE/infrastructure/portal/Dockerfile" "$PAGEHUB_SOURCE"
fi
docker image inspect "$PAGEHUB_CADDY_IMAGE" "$PAGEHUB_PORTAL_IMAGE" >/dev/null
