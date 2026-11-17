#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/common.sh"
require_root
[[ "$(uname -s)" == Linux ]] || die '部署目标必须为 Linux'
[[ "$(uname -m)" == x86_64 || "$(uname -m)" == aarch64 ]] || die '支持 amd64 和 arm64'
[[ -d /run/systemd/system ]] || die '部署目标需要 systemd，请使用 Linux 服务器或 VM'
for tool in flock curl tar sha256sum python3 systemctl ss; do command -v "$tool" >/dev/null || die "缺少工具 $tool"; done
python3 -c 'import sys; sys.exit(sys.version_info < (3, 11))' || die '安装需要 Python 3.11 或更新版本'
command -v docker >/dev/null || bash "$PAGEHUB_SOURCE/scripts/setup-docker.sh"
docker info >/dev/null
docker compose version >/dev/null
mkdir -p "$PAGEHUB_ROOT"
available="$(df -Pk "$PAGEHUB_ROOT" | awk 'NR==2 {print $4}')"
[[ "$available" -ge 2097152 ]] || die '安装目录至少需要 2 GiB 可用空间'
if [[ -n "$(ss -H -ltn "sport = :$PAGEHUB_GATEWAY_PORT")" ]]; then
  die '网关端口已占用，请调整 PAGEHUB_GATEWAY_PORT'
fi
[[ -z "$(ss -H -ltn "sport = :$PAGEHUB_ADMIN_PORT")" ]] || die '本机管理端口已占用，请先调整冲突的服务'
