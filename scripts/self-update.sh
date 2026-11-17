#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/common.sh"
require_root
require_install
audit="$PAGEHUB_ROOT/state/operations/self-update-$(date -u +%Y%m%dT%H%M%S)-$$.json"
install -d -m 0700 "$PAGEHUB_ROOT/state/operations"
write_result() {
  python3 - "$audit" "$1" "$PAGEHUB_ROOT/state/self-request.json" "$PAGEHUB_ROOT/state/self-result.json" "$PAGEHUB_ROOT/state/self-results" <<'PY'
import json, os, sys, datetime, tempfile
record = json.load(open(sys.argv[3]))
record.update(action='update-self', state=sys.argv[2], actor='root', at=datetime.datetime.now(datetime.timezone.utc).isoformat())
record.update(pid=os.getppid(), boot_id=open('/proc/sys/kernel/random/boot_id').read().strip())
if record['state'] == 'error':
    record['error'] = '平台更新失败，请运行 pagehub logs updater 检查独立更新日志'
if not record['id'].isdigit() or len(record['id']) > 32:
    raise SystemExit('独立更新任务 ID 无效')
os.makedirs(sys.argv[5], mode=0o700, exist_ok=True)
for target in (sys.argv[1], sys.argv[4], os.path.join(sys.argv[5], record['id'] + '.json')):
    with tempfile.NamedTemporaryFile(mode='w', dir=os.path.dirname(target), delete=False) as stream:
        json.dump(record, stream)
        stream.flush()
        os.fsync(stream.fileno())
    os.chmod(stream.name, 0o600)
    os.replace(stream.name, target)
PY
}
write_result running
trap 'write_result error' ERR
stage="$(runner desired prepare-self)"
bash "$PAGEHUB_ROOT/current/scripts/update.sh" "$stage"
write_result succeeded
