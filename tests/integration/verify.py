import base64
import hashlib
import json
import copy
import os
import pathlib
import socket
import subprocess
import time
import urllib.error
import urllib.request
import zipfile

root = pathlib.Path(os.environ['PAGEHUB_ROOT'])
port = int(os.environ['PAGEHUB_GATEWAY_PORT'])
base = f'http://127.0.0.1:{port}'
runner_path = str(root / 'current/bin/pagehub-runner')


def runner(*args, ok=True):
    result = subprocess.run([runner_path, *args], capture_output=True, text=True)
    assert (result.returncode == 0) == ok, (args, result.stdout, result.stderr)
    return result.stdout


def request(path, data=None, headers=None, status=200, admin=False):
    url = ('http://127.0.0.1:8091' if admin else base) + path
    req = urllib.request.Request(url, data=data, headers=headers or {})
    try:
        response = urllib.request.urlopen(req, timeout=10)
    except urllib.error.HTTPError as error:
        response = error
    assert response.status == status, (path, response.status, response.read())
    return response.read()


def wait_healthy(app_id):
    for _ in range(50):
        states = json.loads(runner('status'))
        if next(app['state'] for app in states if app['id'] == app_id) == 'healthy':
            return
        time.sleep(1)
    raise AssertionError(f'{app_id} 健康检查超时')


assert b'PageHub' in request('/')
assert b'PageHub' in request('/static-demo/')
assert b'body' in request('/static-demo/style.css')
assert b'PageHub' in request('/static-demo/deep/link')
request('/image-demo/', status=503)
image_record = next(app for app in json.loads((root / 'config/apps.yaml').read_text())['apps'] if app['manifest']['id'] == 'image-demo')
image_runtime = json.loads((pathlib.Path(image_record['directory']) / 'pagehub.runtime.json').read_text())
declared = json.loads(subprocess.check_output(['docker', 'image', 'inspect', image_record['manifest']['source']['image']]))[0]['Config'].get('Volumes') or {}
targets = {mount['target'] for mount in image_runtime['services']['web'].get('volumes', [])}
assert set(declared) <= targets, '镜像声明的数据目录必须映射稳定具名卷'
states = json.loads(runner('status'))
assert next(app['state'] for app in states if app['id'] == 'image-demo') == 'stopped'
runner('recover')
assert next(app['state'] for app in json.loads(runner('status')) if app['id'] == 'image-demo') == 'stopped'
runner('app', 'start', 'image-demo')
wait_healthy('image-demo')
request('/image-demo/')
runner('app', 'stop', 'image-demo')
request('/image-demo/', status=503)

# 持久化数据经过应用启停与默认恢复后保持原值。
request('/compose-demo/api/value', b'persistent-test-value')
assert request('/compose-demo/api/value') == b'persistent-test-value'
assert request('/compose-demo/media', headers={'Range': 'bytes=2-5'}, status=206) == b'2345'
ids_before = subprocess.check_output(['docker', 'ps', '-q'], text=True).split()
runner('recover')
ids_after = subprocess.check_output(['docker', 'ps', '-q'], text=True).split()
assert set(ids_before) == set(ids_after), '幂等恢复重建了健康容器'
runner('app', 'stop', 'compose-demo')
runner('recover')
wait_healthy('compose-demo')
assert request('/compose-demo/api/value') == b'persistent-test-value'

# 数据库保持专网，只有 Web 服务连接共享入口。
db_id = subprocess.check_output(['docker', 'compose', '-p', 'pagehub-compose-demo', '-f', str(next((root / 'apps/compose-demo/releases').glob('*/pagehub.runtime.json'))), 'ps', '-q', 'db'], text=True).strip()
db = json.loads(subprocess.check_output(['docker', 'inspect', db_id]))[0]
assert 'pagehub-ingress' not in db['NetworkSettings']['Networks']
assert not db['HostConfig']['PortBindings']

# WebSocket 升级与双向数据通过共享网关。
with socket.create_connection(('127.0.0.1', port), timeout=5) as connection:
    connection.sendall(b'GET /compose-demo/socket HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n')
    response = b''
    while b'\r\n\r\n' not in response:
        response += connection.recv(1)
    assert b'101 Switching Protocols' in response
    connection.sendall(b'\x81\x82\x01\x02\x03\x04ik')
    assert connection.recv(4) == b'\x81\x02hi'

auth = {'Authorization': 'Bearer test-only-admin-token'}
request('/admin/api/apps', admin=True, status=401)
request('/admin/api/apps', headers={**auth, 'Origin': 'http://evil.example'}, admin=True, status=403)
request('/admin/api/apps/image-demo/start', data=b'', status=403)
request('/admin/api/apps/unknown/start', data=b'', headers=auth, admin=True, status=404)
job = json.loads(request('/admin/api/apps/image-demo/start', data=b'', headers=auth, admin=True, status=202))
for _ in range(30):
    job = json.loads(request('/admin/api/jobs/' + job['id'], headers=auth, admin=True))
    if job['state'] in ['succeeded', 'error']:
        break
    time.sleep(1)
assert job['state'] == 'succeeded', job
runner('app', 'stop', 'image-demo')

# 重复导入明确拒绝，错误摘要拒绝且注册表不变。
registry_before = (root / 'config/apps.yaml').read_bytes()
runner('add-app', '/tmp/pagehub-packages/static-site.zip', '--dry-run', ok=False)
runner('add-app', '/tmp/pagehub-packages/image-web.zip', '--approved-sha256', '0' * 64, ok=False)
assert (root / 'config/apps.yaml').read_bytes() == registry_before

# 静态应用版本更新、回退与私有路由保护。
original = pathlib.Path('/workspace/apps/examples/static-site/app.yaml').read_text()
new_zip = pathlib.Path('/tmp/pagehub-packages/static-v2.zip')
with zipfile.ZipFile(new_zip, 'w') as archive:
    archive.writestr('app.yaml', original.replace('1.0.0', '1.0.1'))
    archive.writestr('dist/index.html', '<h1>version-two</h1>')
runner('update', 'static-demo', str(new_zip), '--approved-sha256', hashlib.sha256(new_zip.read_bytes()).hexdigest())
assert b'version-two' in request('/static-demo/')
runner('rollback', 'static-demo')
assert b'PageHub' in request('/static-demo/')
private_zip = pathlib.Path('/tmp/pagehub-packages/private.zip')
with zipfile.ZipFile(private_zip, 'w') as archive:
    archive.writestr('app.yaml', original.replace('static-demo', 'private-demo').replace('visibility: public', 'visibility: private'))
    archive.writestr('dist/index.html', 'secret-data')
runner('add-app', str(private_zip), '--approved-sha256', hashlib.sha256(private_zip.read_bytes()).hexdigest())
request('/private-demo/', status=403)
assert b'private-demo' not in request('/_pagehub/api/apps')

# 验证备份收据和卷归档，同时确认备份完成后应用恢复。
backup_dir = pathlib.Path(runner('backup', 'compose-demo').strip())
receipt = json.loads((backup_dir / 'receipt.json').read_text())
assert receipt['files']
for name, digest in receipt['files'].items():
    assert hashlib.sha256((backup_dir / name).read_bytes()).hexdigest() == digest
wait_healthy('compose-demo')
assert request('/compose-demo/api/value') == b'persistent-test-value'
runner('backup', 'verify', 'compose-demo', str(backup_dir))
runner('restore', 'compose-demo', str(backup_dir), '--dry-run')
runner('app', 'stop', 'compose-demo')
runner('restore', 'compose-demo', str(backup_dir))
runner('app', 'start', 'compose-demo')
assert json.loads((backup_dir / 'verification.json').read_text())['restore_verified']

runner('backup', 'image-demo', ok=False)


def package(name, manifest, files):
    destination = pathlib.Path('/tmp/pagehub-packages') / (name + '.zip')
    with zipfile.ZipFile(destination, 'w') as archive:
        archive.writestr('app.yaml', json.dumps(manifest, ensure_ascii=False))
        for path, data in files.items():
            archive.writestr(path, data)
    return destination


def approve(command, archive, *extra, ok=True):
    return runner(command, str(archive), *extra, '--approved-sha256', hashlib.sha256(archive.read_bytes()).hexdigest(), ok=ok)


# 镜像声明的 VOLUME 在容器被移除并重新创建后仍使用同一个具名卷。
volume_manifest = copy.deepcopy(image_record['manifest'])
volume_manifest.update(id='volume-demo', path='/volume-demo/')
volume_manifest['source'] = {'image': 'redis@sha256:bb186d083732f669da90be8b0f975a37812b15e913465bb14d845db72a4e3e08', 'container_port': 6379}
volume_zip = package('volume', volume_manifest, {})
approve('add-app', volume_zip)
volume_record = next(app for app in json.loads((root / 'config/apps.yaml').read_text())['apps'] if app['manifest']['id'] == 'volume-demo')
volume_config = str(pathlib.Path(volume_record['directory']) / 'pagehub.runtime.json')
volume_command = ['docker', 'compose', '-p', 'pagehub-volume-demo', '-f', volume_config]
runner('app', 'start', 'volume-demo')
subprocess.run(volume_command + ['exec', '-T', 'web', 'redis-cli', 'SET', 'saved-value', 'volume-survives-recreation'], check=True, capture_output=True)
subprocess.run(volume_command + ['exec', '-T', 'web', 'redis-cli', 'SAVE'], check=True, capture_output=True)
subprocess.run(volume_command + ['down'], check=True, capture_output=True)
runner('app', 'start', 'volume-demo')
assert subprocess.check_output(volume_command + ['exec', '-T', 'web', 'redis-cli', 'GET', 'saved-value']).strip() == b'volume-survives-recreation'
runner('remove-app', 'volume-demo')


# 前缀保留交给后端处理，Caddy 欢迎页只存在于内部根路径。
preserve = copy.deepcopy(image_record['manifest'])
preserve.update(id='preserve-demo', path='/preserve-demo/', route_mode='preserve_prefix')
preserve_zip = package('preserve', preserve, {})
approve('add-app', preserve_zip)
runner('app', 'start', 'preserve-demo')
for attempt in range(20):
    try:
        request('/preserve-demo/', status=404)
        break
    except AssertionError:
        if attempt == 19:
            raise
        time.sleep(1)
runner('app', 'stop', 'preserve-demo')

# 故障候选不会修改已生效的登记和路由。
before_registry = (root / 'config/apps.yaml').read_bytes()
before_routes = (root / 'gateway/Caddyfile').read_bytes()
failed = copy.deepcopy(image_record['manifest'])
failed.update(id='unhealthy-demo', path='/unhealthy-demo/', startup='always')
failed['source']['container_port'] = 81
failed_zip = package('unhealthy', failed, {})
approve('add-app', failed_zip, ok=False)
assert (root / 'config/apps.yaml').read_bytes() == before_registry
assert (root / 'gateway/Caddyfile').read_bytes() == before_routes
assert not subprocess.check_output(['docker', 'ps', '-aq', '--filter', 'label=com.docker.compose.project=pagehub-unhealthy-demo']).strip()
assert request('/compose-demo/api/value') == b'persistent-test-value'

failed.update(id='gateway-fail', path='/gateway-fail/', startup='manual')
gateway_zip = package('gateway-fail', failed, {})
env = dict(os.environ, PAGEHUB_CADDY_IMAGE='sha256:' + '0' * 64)
result = subprocess.run([runner_path, 'add-app', str(gateway_zip), '--approved-sha256', hashlib.sha256(gateway_zip.read_bytes()).hexdigest()], env=env, capture_output=True)
assert result.returncode != 0
assert (root / 'config/apps.yaml').read_bytes() == before_registry
assert (root / 'gateway/Caddyfile').read_bytes() == before_routes

# 容器版本更新和经维护收据授权的回退，继续使用原具名卷。
record = next(app for app in json.loads(before_registry)['apps'] if app['manifest']['id'] == 'compose-demo')
runtime = json.loads((pathlib.Path(record['directory']) / 'pagehub.runtime.json').read_text())
compose = copy.deepcopy(runtime)
compose['networks'].pop('pagehub-ingress')
for service in compose['services'].values():
    service.pop('build', None)
    service.get('networks', {}).pop('pagehub-ingress', None)
new_manifest = copy.deepcopy(record['manifest'])
new_manifest['version'] = '1.0.1'
new_zip = package('compose-v2', new_manifest, {'compose.yaml': json.dumps(compose)})
maintenance = pathlib.Path('/tmp/pagehub-maintenance.json')
maintenance.write_text(json.dumps({'id': 'compose-demo', 'digest': record['digest'], 'backup': str(backup_dir / 'receipt.json'), 'restore_verified': True, 'expires_at': '2099-01-01T00:00:00Z'}))
runner('update', 'compose-demo', str(new_zip), '--maintenance-receipt', str(maintenance), '--approved-sha256', hashlib.sha256(new_zip.read_bytes()).hexdigest())
assert request('/compose-demo/api/value') == b'persistent-test-value'
updated = next(app for app in json.loads((root / 'config/apps.yaml').read_text())['apps'] if app['manifest']['id'] == 'compose-demo')
assert updated['manifest']['version'] == '1.0.1'
maintenance.write_text(json.dumps({'id': 'compose-demo', 'digest': updated['digest'], 'backup': str(backup_dir / 'receipt.json'), 'restore_verified': True, 'expires_at': '2099-01-01T00:00:00Z'}))
runner('rollback', 'compose-demo', '--maintenance-receipt', str(maintenance))
assert request('/compose-demo/api/value') == b'persistent-test-value'

# 接管真实既有工程，保留原工程名、环境变量与数据卷。
legacy = json.loads(json.dumps(compose).replace('pagehub-compose-demo', 'pagehub-test-legacy'))
legacy['services']['web']['environment'] = {'PRESERVED_TEST_SETTING': 'keep-this-value'}
legacy_file = pathlib.Path('/tmp/legacy-compose.json')
legacy_file.write_text(json.dumps(legacy))
subprocess.run(['docker', 'compose', '-p', 'pagehub-test-legacy', '-f', str(legacy_file), 'up', '-d', '--no-build', '--pull', 'never', '--wait'], check=True, capture_output=True)
subprocess.run(['docker', 'compose', '-p', 'pagehub-test-legacy', '-f', str(legacy_file), 'exec', '-T', 'db', 'redis-cli', 'SET', 'pagehub-example', 'adopted-persistent-value'], check=True, capture_output=True)
adoption_config = copy.deepcopy(legacy)
adoption_config['services']['web'].pop('environment')
adoption = copy.deepcopy(record['manifest'])
adoption.update(id='adopt-demo', path='/adopt-demo/')
adoption_zip = package('adoption', adoption, {'compose.yaml': json.dumps(adoption_config)})
runner('adopt-app', str(adoption_zip), '--project-name', 'pagehub-test-legacy', '--dry-run')
approve('adopt-app', adoption_zip, '--project-name', 'pagehub-test-legacy')
assert request('/adopt-demo/api/value') == b'adopted-persistent-value'
adopted = next(app for app in json.loads((root / 'config/apps.yaml').read_text())['apps'] if app['manifest']['id'] == 'adopt-demo')
assert adopted['project'] == 'pagehub-test-legacy' and adopted['adopted']
web_id = subprocess.check_output(['docker', 'compose', '-p', 'pagehub-test-legacy', '-f', str(legacy_file), 'ps', '-q', 'web'], text=True).strip()
web = json.loads(subprocess.check_output(['docker', 'inspect', web_id]))[0]
assert 'PRESERVED_TEST_SETTING=keep-this-value' in web['Config']['Env']
assert b'keep-this-value' not in request('/_pagehub/api/apps')

# 安装信任清单只接入显式批准包，重复运行保持幂等。
bundled = copy.deepcopy(image_record['manifest'])
bundled.update(id='bundled-demo', name='内置包示例', path='/bundled-demo/')
bundled['startup'] = 'manual'
bundled_zip = package('bundled', bundled, {})
bundled_directory = root / 'current/apps/bundled'
bundled_target = bundled_directory / 'approved.zip'
bundled_target.write_bytes(bundled_zip.read_bytes())
policy_file = root / 'current/config/policies/bundled-apps.json'
policy_file.write_text(json.dumps({'packages': [
    {'id': 'bundled-demo', 'file': 'approved.zip', 'sha256': hashlib.sha256(bundled_zip.read_bytes()).hexdigest(), 'enabled': True},
    {'id': 'disabled-demo', 'file': 'never-execute.zip', 'sha256': '0' * 64, 'enabled': False},
]}))
subprocess.run(['bash', str(root / 'current/scripts/import-bundled-apps.sh')], check=True, capture_output=True)
registered = (root / 'config/apps.yaml').read_bytes()
subprocess.run(['bash', str(root / 'current/scripts/import-bundled-apps.sh')], check=True, capture_output=True)
assert (root / 'config/apps.yaml').read_bytes() == registered
assert b'bundled-demo' in registered and b'disabled-demo' not in registered
print('PASS 三类适配器 / API / 任务 / WebSocket / Range / 持久化 / 健康失败回退 / 网关失败 / 更新回滚 / 私有路由 / 备份 / 显式接管')
