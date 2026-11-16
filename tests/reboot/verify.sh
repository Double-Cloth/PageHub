#!/usr/bin/env bash
set -Eeuo pipefail
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)/scripts/lib/common.sh"
require_root
require_install
[[ "${1:-}" == prepare || "${1:-}" == verify ]] || die '用法: tests/reboot/verify.sh prepare|verify，仅用于安装了示例夹具的隔离 VM'
python3 - "$1" <<'PY'
import hashlib
import json
import os
import pathlib
import subprocess
import sys
import time
import urllib.request

root = pathlib.Path(os.environ['PAGEHUB_ROOT'])
base = 'http://127.0.0.1:' + os.environ['PAGEHUB_GATEWAY_PORT']
baseline = root / 'state/reboot-baseline.json'
boot_id = pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()
binary = str(root / 'current/bin/pagehub-runner')
records = json.load(open(root / 'config/apps.yaml'))['apps']
fixture = next((app for app in records if app['manifest']['id'] == 'compose-demo'), None)
if not fixture:
    raise SystemExit('需要先在隔离 VM 导入 Compose 示例，用其数据库检查持久化')

if sys.argv[1] == 'prepare':
    value = ('pagehub-reboot-' + str(time.time_ns())).encode()
    urllib.request.urlopen(urllib.request.Request(base + '/compose-demo/api/value', data=value), timeout=10).read()
    volumes = subprocess.check_output(['docker', 'volume', 'ls', '--filter', 'label=com.docker.compose.project=' + fixture['project'], '--format', '{{.Name}}'], text=True).split()
    baseline.write_text(json.dumps({'boot_id': boot_id, 'sha256': hashlib.sha256(value).hexdigest(), 'volumes': sorted(volumes)}))
    print('基线已保存。现在在此隔离 VM 执行真实 reboot，重连后执行 verify。')
else:
    expected = json.loads(baseline.read_text())
    if boot_id == expected['boot_id']:
        raise SystemExit('boot_id 未变化，不能将本轮检查标记为整机重启验收')
    for _ in range(60):
        states = json.loads(subprocess.check_output([binary, 'status']))
        statuses = {app['id']: app['state'] for app in states}
        ready = all(statuses.get(app['manifest']['id']) == 'healthy' for app in records if app['manifest']['startup'] == 'always' and app['manifest']['visibility'] == 'public')
        if ready:
            break
        time.sleep(2)
    else:
        raise SystemExit('常驻应用开机恢复超时')
    for app in records:
        if app['manifest']['startup'] == 'manual' and statuses.get(app['manifest']['id']) != 'stopped':
            raise SystemExit('manual 应用被意外启动')
    value = urllib.request.urlopen(base + '/compose-demo/api/value', timeout=10).read()
    assert hashlib.sha256(value).hexdigest() == expected['sha256'], '重启后数据摘要变化'
    volumes = subprocess.check_output(['docker', 'volume', 'ls', '--filter', 'label=com.docker.compose.project=' + fixture['project'], '--format', '{{.Name}}'], text=True).split()
    assert sorted(volumes) == expected['volumes'], '重启后持久卷引用变化'
    subprocess.run(['systemctl', 'is-active', '--quiet', 'pagehub.service', 'pagehub-runner.service', 'pagehub-manager.service'], check=True)
    print('PASS 真实整机重启、自动恢复、manual 策略和数据持久化')
PY
