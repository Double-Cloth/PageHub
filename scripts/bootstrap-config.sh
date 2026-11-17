#!/usr/bin/env bash
# 供 install.sh 加载，在解析用户配置前准备已有 Go Runner。
[[ -n "${config_file:-}" && -f "$config_file" ]] || die '需要存在的 pagehub.yaml'
command -v git >/dev/null || die '缺少 git，请先安装 git 后重试'
command -v docker >/dev/null || bash "$PAGEHUB_SOURCE/scripts/setup-docker.sh"
docker info >/dev/null
bootstrap_dir="$(mktemp -d)"
trap 'rm -rf -- "$bootstrap_dir"' EXIT
if [[ "$PAGEHUB_OFFLINE" == true ]]; then
  bootstrap_runner="$PAGEHUB_SOURCE/bin/pagehub-runner"
  [[ -x "$bootstrap_runner" ]] || die '离线包缺少 pagehub-runner'
else
  docker run --rm -v "$PAGEHUB_SOURCE:/src:ro" -v "$bootstrap_dir:/out" -w /src/backend -e CGO_ENABLED=0 golang:1.25.4-alpine go build -trimpath -o /out/pagehub-runner ./cmd/runner
  bootstrap_runner="$bootstrap_dir/pagehub-runner"
fi
"$bootstrap_runner" desired bootstrap-env -c "$config_file" > "$bootstrap_dir/config.env"
set -a
source "$bootstrap_dir/config.env"
set +a
