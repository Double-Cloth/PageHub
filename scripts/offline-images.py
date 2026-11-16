"""收集当前已批准部署的实际镜像和原始引用，供离线导入恢复解析。"""
import json
import os
import pathlib
import subprocess
import sys

root, source, output = map(pathlib.Path, sys.argv[1:])
registry = json.load(open(root / 'config/apps.yaml', encoding='utf-8'))
records = {app['manifest']['id']: app for app in registry['apps']}
bundled = json.load(open(source / 'config/policies/bundled-apps.json', encoding='utf-8'))
for item in bundled['packages']:
    if item.get('enabled'):
        current = records.get(item['id'])
        if not current or current['digest'] != item['sha256']:
            raise SystemExit('离线打包前必须在构建机导入并验证每个启用的内置包')

mapping = {}
for app in registry['apps']:
    manifest = app['manifest']
    if manifest['kind'] == 'static':
        continue
    directory = pathlib.Path(app['directory'])
    runtime = json.load(open(directory / 'pagehub.runtime.json', encoding='utf-8'))
    if manifest['kind'] == 'image':
        originals = {'web': {'image': manifest['source']['image']}}
    else:
        raw = subprocess.check_output([
            'docker', 'compose', '--project-name', app['project'],
            '--project-directory', str(directory), '-f',
            str(directory / manifest['source']['compose_file']),
            'config', '--format', 'json',
        ], env={key: os.environ[key] for key in ['PATH', 'HOME', 'DOCKER_HOST', 'DOCKER_CONTEXT', 'DOCKER_CONFIG', 'SYSTEMROOT'] if key in os.environ})
        originals = json.loads(raw)['services']
    for name, service in runtime['services'].items():
        image_id = service['image']
        original = originals[name].get('image', app['project'] + '-' + name)
        if original in mapping and mapping[original] != image_id:
            raise SystemExit('同一原始镜像引用对应多个版本，请分开构建离线发布包')
        mapping[original] = image_id
        mapping[image_id] = image_id

output.write_text(json.dumps(mapping, indent=2) + '\n', encoding='utf-8')
for image_id in sorted(set(mapping.values())):
    print(image_id)
