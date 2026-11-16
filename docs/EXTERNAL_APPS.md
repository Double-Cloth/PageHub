# 独立项目接入

标准 Static、Image、Compose 包继续使用原导入方式。外部项目使用仅包含 app.yaml 的归档，经 add-app 的摘要审批流程登记。

## 外部服务

```yaml
schema_version: 1
id: existing-app
name: 既有应用
version: 1.0.0
kind: external
path: /existing-app/
startup: manual
visibility: public
route_mode: strip_prefix
source:
  upstream: http://existing-web:8080
lifecycle:
  allow: []
backup:
  mode: external
```

upstream 必须为共享网关容器可以访问的 HTTP(S) Origin。宿主机 127.0.0.1 在容器内指向容器自身，应使用实际可达的地址。PageHub 提供目录、反向代理和经网关的健康检查，服务继续使用原部署工具维护。

## 既有 Compose

将 kind 改为 external-compose，在 source 中增加 compose_directory、compose_file、project、config_sha256 和 approvals。目录使用 Linux 绝对路径，project 使用原项目名。lifecycle.allow 可按需授予 start、stop、restart。

PageHub 对原工程执行 start/stop，restart 先停止再启动已有容器。接入前须由原部署工具创建容器，并确保 Web 入口已能被网关访问。接入保留原卷、服务、网络和重启策略。删除登记会移除路由，容器和卷继续保留。

使用与部署相同的项目名、目录及配置文件运行 docker compose config --format json。解析结果经 Go encoding/json.Marshal 规范化后计算 SHA-256，填入 config_sha256。每次受控操作重新解析和复核摘要，环境配置变化后须重新审核。

approvals 按服务和字段逐项填写解析后的精确值。例如 web.ports、web.profiles、web.entrypoint、web.volumes、web.cap_add、web.devices、web.privileged。所有挂载以及标准白名单以外字段都必须明确批准。完整摘要同时绑定顶层卷、网络和全部服务配置。审批可能包含敏感路径与配置，应妥善保管。

外部模式的更新与备份使用原部署工具。修改接入描述时先移除登记，再导入重新审批的描述包。

运行 `sudo bash pagehub.sh compose-review /srv/existing compose.yaml existing` 可生成 config_sha256 与逐字段 approvals 候选值。应逐项检查后复制至描述包，该命令的输出可能含敏感配置。
