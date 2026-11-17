"""通过真实 Runner、Docker 和网关验证托管事务，Git 源使用隔离夹具。"""
import json
import os
from pathlib import Path
import subprocess
import shutil
import tempfile
import time
import urllib.error
import urllib.request


root = Path(os.environ["PAGEHUB_ROOT"])
runner = root / "current/bin/pagehub-runner"
origin = "http://127.0.0.1:" + os.environ["PAGEHUB_GATEWAY_PORT"]
fixture = Path(tempfile.mkdtemp(prefix="pagehub-managed-fixture-"))
state = fixture / "versions.json"
state.write_text(json.dumps({"static": "a" * 40, "diary": "c" * 40}))
wrapper = fixture / "git"
wrapper.write_text("""#!/usr/bin/python3
import json, pathlib, sys, zipfile
base = pathlib.Path(%r)
args = sys.argv[1:]
versions = json.loads((base / 'versions.json').read_text())
if args[0] == 'ls-remote':
    kind = 'static' if 'Whalgebra' in args[-2] else 'diary'
    print(versions[kind] + '\\t' + args[-1])
elif args[0] == 'init':
    pathlib.Path(args[-1]).mkdir(parents=True)
elif args[0] == 'archive':
    output = next(arg.split('=', 1)[1] for arg in args if arg.startswith('--output='))
    commit = args[args.index('--format=zip') + 2]
    source = base / commit
    with zipfile.ZipFile(output, 'w', zipfile.ZIP_DEFLATED) as archive:
        for file in source.rglob('*'):
            if file.is_file(): archive.write(file, file.relative_to(source))
elif args[0] not in ('fetch', 'cat-file'):
    raise SystemExit('unexpected git command')
""" % str(fixture))
wrapper.chmod(0o755)
environment = {**os.environ, "PATH": str(fixture) + ":" + os.environ["PATH"]}


def run(*args, expected=True, input=None):
    result = subprocess.run([str(runner), *args], env=environment, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, input=input, timeout=240)
    if expected and result.returncode:
        raise AssertionError(result.stderr)
    if not expected and result.returncode == 0:
        raise AssertionError("操作应失败")
    return result.stdout


def docker(*args):
    return subprocess.check_output(["docker", *args], text=True, timeout=180).strip()


def status_code(path):
    try:
        return urllib.request.urlopen(origin + path, timeout=5).status
    except urllib.error.HTTPError as error:
        return error.code


for commit, text in [("a" * 40, "managed-v1"), ("b" * 40, "managed-v2")]:
    directory = fixture / commit / "dist"
    directory.mkdir(parents=True)
    (directory / "Whalgebra.html").write_text("<!doctype html><title>Calc</title>" + text)

# 只把已由单元测试校验的示例转换成 JSON，避免测试工具新增 YAML 依赖。
config = {
    "schema_version": 1,
    "platform": {"name": "PageHub", "repository": "https://github.com/double-cloth-studio/PageHub.git",
                 "branch": "main", "root": str(root), "source_dir": str(root / "sources/pagehub"),
                 "public_origin": "https://example.test:63445", "gateway": {"bind": "127.0.0.1", "port": int(os.environ["PAGEHUB_GATEWAY_PORT"])},
                 "admin": {"bind": "127.0.0.1", "port": 8091},
                 "tunnel": {"provider": "sakurafrp", "mode": "external", "local_target": origin.removeprefix("http://")}},
    "apps": [{"id": "managed-calc", "name": "Managed Calc", "enabled": True, "path": "/managed-calc/",
              "source": {"type": "git", "repository": "https://github.com/double-cloth-studio/Whalgebra.git", "branch": "main"},
              "deployment": {"driver": "managed-static", "publish": {"source_file": "dist/Whalgebra.html", "target_file": "index.html"}, "route_mode": "strip_prefix"},
              "persistence": {"kind": "none"}, "startup": "always"}]
}
file = fixture / "pagehub.json"


def apply():
    file.write_text(json.dumps(config))
    review = json.loads(run("desired", "plan", "-c", str(file)))
    run("desired", "apply", "-c", str(file), "--approved-plan", review["sha256"])


def record(app_id):
    return next(app for app in json.loads((root / "config/apps.yaml").read_text())["apps"]
                if app["manifest"]["id"] == app_id)


apply()
first = record("managed-calc")
assert first["managed"]["commit"] == "a" * 40
catalog = json.load(urllib.request.urlopen(origin + "/_pagehub/api/apps"))
entry = next(app for app in catalog if app["id"] == "managed-calc")
assert entry["displayReady"] and not entry["offlineReady"]
apply()
assert record("managed-calc")["digest"] == first["digest"]
run("app", "stop", "managed-calc")
assert status_code("/managed-calc/") == 503
run("app", "start", "managed-calc")
assert status_code("/managed-calc/") == 200
state.write_text(json.dumps({"static": "b" * 40, "diary": "c" * 40}))
review = json.loads(run("desired", "update", "managed-calc", "--check"))
run("desired", "update", "managed-calc", "--approved-plan", review["sha256"])
assert record("managed-calc")["managed"]["commit"] == "b" * 40
run("rollback", "managed-calc")
assert record("managed-calc")["managed"]["commit"] == "a" * 40

image = os.environ.get("PAGEHUB_TEST_DIARY_IMAGE")
if image:
    source = fixture / ("c" * 40)
    source.mkdir()
    (source / "Dockerfile").write_text("FROM " + image + "\n")
    (source / "compose.yaml").write_text(json.dumps({
        "services": {"app": {"build": ".", "image": "${TRAVEL_IMAGE}", "init": True,
                             "environment": {"TRAVEL_LOCAL": "false", "TRAVEL_PORT": "9000", "TRAVEL_MAX_PORT_RETRIES": "1", "SITE_DOMAIN": "${SITE_DOMAIN}", "SITE_SCHEME": "${SITE_SCHEME}", "TRAVEL_PUBLIC_ORIGIN": "${TRAVEL_PUBLIC_ORIGIN}", "TRAVEL_BASE_PATH": "${TRAVEL_BASE_PATH}", "TRAVEL_WRITE_MODE": "${TRAVEL_WRITE_MODE}"},
                             "volumes": ["runtime:/var/lib/travel-diary"], "expose": ["9000"]}},
        "volumes": {"runtime": {}}
    }))
    (source / "compose.build.yaml").write_text('{"services":{"app":{"build":"."}}}')
    (source / "compose.portal.yaml").write_text(json.dumps({"services": {"app": {"networks": {"default": {}, "portal": {"aliases": ["${TRAVEL_PORTAL_ALIAS}"]}}}}, "networks": {"portal": {"external": True, "name": "${TRAVEL_PORTAL_NETWORK}"}}}))
    docker("volume", "create", "travel-managed-test_runtime")
    code = 'const a=require("/opt/travel-diary/js/auth.js"),crypto=require("node:crypto");require("/opt/travel-diary/js/server.js").ensureDataStructure("/var/lib/travel-diary").then(async()=>{let config;for(let i=0;i<10&&!config;i++){try{config=await a.createAuthConfig(Array.from(crypto.randomBytes(a.PASSWORD_LENGTH),n=>n%10).join(""))}catch{}}if(!config)throw Error("test auth initialization failed");await a.writeAuthConfig("/var/lib/travel-diary",config)}).catch(e=>{console.error(e.message);process.exit(1)})'
    docker("run", "--rm", "-v", "travel-managed-test_runtime:/var/lib/travel-diary", image, "node", "-e", code)
    config["apps"].append({"id": "managed-diary", "name": "Managed Diary", "enabled": True, "path": "/managed-diary/", "source": {"type": "git", "repository": "https://github.com/double-cloth-studio/travel-diary.git", "branch": "main"}, "deployment": {"driver": "managed-compose", "project": "travel-managed-test", "compose_files": ["compose.yaml", "compose.build.yaml", "compose.portal.yaml"], "service": "app", "upstream_port": 9000, "ingress_alias": "managed-diary", "route_mode": "strip_prefix"}, "persistence": {"kind": "compose-volume", "volume": "runtime", "backup_driver": "travel-diary-native", "retain_on_uninstall": True}, "startup": "always"})
    apply()
    diary = record("managed-diary")
    assert diary["project"] == "travel-managed-test"
    auth = docker("run", "--rm", "-v", "travel-managed-test_runtime:/var/lib/travel-diary", image, "sha256sum", "/var/lib/travel-diary/.secrets/auth.json")
    backup = run("backup", "managed-diary").strip()
    run("backup", "verify", "managed-diary", backup)
    for action in ["stop", "start", "restart"]:
        run("app", action, "managed-diary")
    assert auth == docker("run", "--rm", "-v", "travel-managed-test_runtime:/var/lib/travel-diary", image, "sha256sum", "/var/lib/travel-diary/.secrets/auth.json")

    # 通过真实 Manager/Runner 异步 API 执行原生备份。
    password = "managed-integration-password"
    subprocess.run([str(root / "current/bin/pagehub-manager"), "set-password"], input=password + "\n", text=True, check=True, stdout=subprocess.PIPE)
    def admin(path, body=None):
        headers = {"Authorization": "Bearer test-only-admin-token", "Content-Type": "application/json"}
        payload = json.dumps(body).encode() if body is not None else None
        request = urllib.request.Request("http://127.0.0.1:8091/admin/api/" + path, payload, headers)
        return json.load(urllib.request.urlopen(request, timeout=30))
    job = admin("apps/managed-diary/backup", {"confirm_id": "managed-diary", "password": password})
    deadline = time.monotonic() + 120
    while job["state"] in ("queued", "running"):
        assert time.monotonic() < deadline, "异步备份超时"
        time.sleep(1)
        job = admin("jobs/" + job["id"])
    assert job["state"] == "succeeded", job
    assert job["result"]["backup"]
    assert password not in json.dumps(job)

    # 使用真实原生恢复器验证密码和默认业务数据恢复。
    run("app", "stop", "managed-diary")
    docker("run", "--rm", "-v", "travel-managed-test_runtime:/var/lib/travel-diary", image, "node", "-e", code)
    run("restore", "managed-diary", backup, input="managed-diary\nRESTORE\n")
    assert auth == docker("run", "--rm", "-v", "travel-managed-test_runtime:/var/lib/travel-diary", image, "sha256sum", "/var/lib/travel-diary/.secrets/auth.json")
    run("app", "start", "managed-diary")

    upgraded = fixture / ("d" * 40)
    shutil.copytree(source, upgraded)
    (upgraded / "Dockerfile").write_text("FROM " + image + "\nLABEL pagehub.test.version=2\n")
    state.write_text(json.dumps({"static": "b" * 40, "diary": "d" * 40}))
    review = json.loads(run("desired", "update", "managed-diary", "--check"))
    run("desired", "update", "managed-diary", "--approved-plan", review["sha256"])
    current = record("managed-diary")
    assert current["managed"]["commit"] == "d" * 40
    assert current["managed"]["image"] != diary["managed"]["image"]
    run("rollback", "managed-diary")
    assert record("managed-diary")["managed"]["commit"] == "c" * 40
    run("rollback", "managed-diary")
    assert record("managed-diary")["managed"]["commit"] == "d" * 40

    broken = fixture / ("e" * 40)
    shutil.copytree(upgraded, broken)
    (broken / "Dockerfile").write_text("FROM " + image + "\nLABEL pagehub.test.version=broken\n")
    failing = json.loads((broken / "compose.yaml").read_text())
    failing["services"]["app"]["command"] = ["node", "-e", "process.exit(1)"]
    (broken / "compose.yaml").write_text(json.dumps(failing))
    state.write_text(json.dumps({"static": "b" * 40, "diary": "e" * 40}))
    review = json.loads(run("desired", "update", "managed-diary", "--check"))
    run("desired", "update", "managed-diary", "--approved-plan", review["sha256"], expected=False)
    assert record("managed-diary")["managed"]["commit"] == "d" * 40
    for _ in range(30):
        if status_code("/managed-diary/readyz") == 200: break
        time.sleep(1)
    assert status_code("/managed-diary/readyz") == 200
    assert auth == docker("run", "--rm", "-v", "travel-managed-test_runtime:/var/lib/travel-diary", image, "sha256sum", "/var/lib/travel-diary/.secrets/auth.json")
    run("desired", "uninstall", "managed-diary")
    docker("volume", "inspect", "travel-managed-test_runtime")
    assert Path(backup, "receipt.json").exists()

    # 重新接管保留数据的独立工程，审批摘要绑定真实原镜像和卷。
    docker("compose", "--project-name", "travel-managed-test", "-f", str(Path(current["directory"], "pagehub.runtime.json")), "up", "-d", "--no-build", "--pull", "never")
    state.write_text(json.dumps({"static": "b" * 40, "diary": "d" * 40}))
    review = json.loads(run("desired", "plan", "--adopt", "-c", str(file)))
    run("desired", "apply", "--adopt", "-c", str(file), "--approved-plan", review["sha256"])
    assert record("managed-diary")["project"] == "travel-managed-test"
    verified = run("backup", "managed-diary").strip()
    run("desired", "uninstall", "managed-diary", "--purge-data", "--backup", verified, input="wrong\n", expected=False)
    docker("volume", "inspect", "travel-managed-test_runtime")
    run("desired", "uninstall", "managed-diary", "--purge-data", "--backup", verified, input="managed-diary\ntravel-managed-test_runtime\n")
    assert subprocess.run(["docker", "volume", "inspect", "travel-managed-test_runtime"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode != 0
    assert Path(verified, "receipt.json").exists()

run("desired", "uninstall", "managed-calc")
assert not (root / "apps/managed-calc/releases").exists()
print("托管 plan/apply 幂等、静态启停、SHA 更新与回滚、默认卸载验证通过")
if image:
    print("真实 Diary 镜像初始化、原生备份恢复、CLI/API、固定卷启停、升级回滚、健康失败数据恢复、接管与双重确认清理验证通过")
