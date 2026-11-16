#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/common.sh"
require_root
runner doctor
systemctl --no-pager status pagehub.service pagehub-manager.service pagehub-runner.service
