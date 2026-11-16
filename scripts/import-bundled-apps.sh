#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/common.sh"
[[ "${PAGEHUB_IMPORT_BUNDLED:-true}" == true ]] || exit 0
manifest="$PAGEHUB_ROOT/current/config/policies/bundled-apps.json"
python3 - "$manifest" "$PAGEHUB_ROOT/current" <<'PY'
import hashlib, json, os, pathlib, subprocess, sys
manifest, release = sys.argv[1:]
for item in json.load(open(manifest, encoding='utf-8'))['packages']:
    if not item.get('enabled', False):
        continue
    path = pathlib.Path(release, 'apps/bundled', item['file']).resolve()
    root = pathlib.Path(release, 'apps/bundled').resolve()
    if not path.is_relative_to(root) or not path.is_file():
        raise SystemExit('内置包路径非法')
    digest = hashlib.file_digest(open(path, 'rb'), 'sha256').hexdigest()
    if digest != item['sha256']:
        raise SystemExit('内置包摘要不匹配')
    registry = json.load(open(os.path.join(os.environ['PAGEHUB_ROOT'], 'config/apps.yaml')))
    existing = next((a for a in registry['apps'] if a['manifest']['id'] == item['id']), None)
    if existing:
        if existing['digest'] != digest:
            raise SystemExit('内置包已登记且版本不同，请显式 update')
        continue
    subprocess.run([os.path.join(release, 'bin/pagehub-runner'), 'add-app', str(path), '--expected-id', item['id'], '--approved-sha256', digest], check=True)
PY
