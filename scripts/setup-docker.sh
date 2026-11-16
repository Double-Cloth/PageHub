#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/common.sh"
require_root
[[ "${PAGEHUB_INSTALL_DOCKER:-false}" == true ]] || die '未发现 Docker。请按 docs.docker.com/engine/install/ 安装，或显式设置 PAGEHUB_INSTALL_DOCKER=true'
source /etc/os-release
[[ "$ID" == ubuntu || "$ID" == debian ]] || die '自动安装 Docker 仅支持 Ubuntu/Debian'
apt-get update
apt-get install -y ca-certificates curl
install -m 0755 -d /etc/apt/keyrings
curl -fsSL "https://download.docker.com/linux/$ID/gpg" -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
codename="${UBUNTU_CODENAME:-$VERSION_CODENAME}"
printf 'Types: deb\nURIs: https://download.docker.com/linux/%s\nSuites: %s\nComponents: stable\nArchitectures: %s\nSigned-By: /etc/apt/keyrings/docker.asc\n' "$ID" "$codename" "$(dpkg --print-architecture)" > /etc/apt/sources.list.d/docker.sources
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker
