"""通过真实网关、Manager 与 Social 验证 UI 配套接口。"""
import http.cookiejar
import json
import os
import pathlib
import subprocess
import urllib.error
import urllib.request

root = pathlib.Path(os.environ["PAGEHUB_ROOT"])
public = "http://127.0.0.1:" + os.environ["PAGEHUB_GATEWAY_PORT"]
admin = "http://127.0.0.1:8091"
browser = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))


def call(origin, path, method="GET", data=None, csrf="", expected=200):
    headers = {"Origin": origin, "Content-Type": "application/json"}
    if csrf:
        headers["X-CSRF-Token"] = csrf
    request = urllib.request.Request(
        origin + path,
        data=None if data is None else json.dumps(data).encode(),
        headers=headers,
        method=method,
    )
    try:
        response = browser.open(request, timeout=15)
    except urllib.error.HTTPError as error:
        response = error
    body = json.loads(response.read())
    assert response.code == expected, (path, response.code, body)
    return body


password = "ui-integration-test-password"
subprocess.run([str(root / "current/bin/pagehub-manager"), "set-password"], input=password + "\n", text=True, check=True, stdout=subprocess.DEVNULL)
portal = call(public, "/_pagehub/api/portal")
assert portal["projects"] == [], "未通过离线验证的应用进入了项目空间"
call(admin, "/admin/api/portal", "PUT", portal, expected=401)
call(admin, "/admin/api/session", "POST", {"password": "incorrect"}, expected=401)
csrf = call(admin, "/admin/api/session", "POST", {"password": password})["csrf"]
portal = call(admin, "/admin/api/portal", csrf=csrf)
portal["profile"]["signature"] = "UI integration verified."
saved = call(admin, "/admin/api/portal", "PUT", portal, csrf=csrf)
assert saved["revision"] == portal["revision"] + 1
call(admin, "/admin/api/portal", "PUT", portal, csrf=csrf, expected=409)
assert call(public, "/_pagehub/api/portal")["profile"]["signature"] == "UI integration verified."
message = call(public, "/_pagehub/social/messages", "POST", {"content": "真实服务持久化弹幕验收"}, expected=201)
call(public, "/_pagehub/social/messages", "POST", {"content": "冷却测试"}, expected=429)
assert any(item["id"] == message["id"] for item in call(public, "/_pagehub/social/messages")["messages"])
call(admin, "/admin/api/social/settings", "PUT", {"enabled": False}, csrf=csrf)
assert call(public, "/_pagehub/social/messages")["enabled"] is False
call(admin, "/admin/api/social/settings", "PUT", {"enabled": True}, csrf=csrf)
audit = call(admin, "/admin/api/audit", csrf=csrf)
assert any(item["action"] == "portal.update" for item in audit)
call(admin, "/admin/api/session/logout", "POST", {}, csrf=csrf)
call(admin, "/admin/api/portal", csrf=csrf, expected=401)
print("真实 UI 配套接口验收通过")
