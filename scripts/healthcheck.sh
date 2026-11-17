#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/common.sh"
failed=0
curl --fail --silent --show-error --retry 10 --retry-delay 1 --retry-all-errors --max-time 5 "http://127.0.0.1:$PAGEHUB_GATEWAY_PORT/healthz" >/dev/null || failed=1
curl --fail --silent --show-error --retry 10 --retry-delay 1 --retry-all-errors --max-time 5 "http://127.0.0.1:$PAGEHUB_GATEWAY_PORT/" >/dev/null || failed=1
curl --fail --silent --show-error --retry 10 --retry-delay 1 --retry-all-errors --max-time 5 "http://127.0.0.1:$PAGEHUB_ADMIN_PORT/_pagehub/api/health" >/dev/null || failed=1
curl --fail --silent --show-error --unix-socket /run/pagehub-social/public.sock http://localhost/_pagehub/social/health >/dev/null || failed=1
runner recover --check || failed=1
exit "$failed"
