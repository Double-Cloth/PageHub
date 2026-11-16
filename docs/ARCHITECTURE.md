# 架构

公网 HTTPS 由已有 SakuraFrp 隧道终止并转发到宿主机回环入口。共享 Caddy 使用 `pagehub-ingress` 连接独立 Portal 和各应用的 Web 服务。数据库保留应用自己的网络。

```text
浏览器 → SakuraFrp → 127.0.0.1:8080 → Caddy → Portal / 应用
                                           ↓
                              Manager 公开只读 Unix Socket
SSH 转发 → 127.0.0.1:8091 → Manager → Runner Unix Socket → Docker
```

`backend/internal/manifests` 负责严格 YAML 清单，`adapters` 负责三类应用与 Compose 权限，`importer` 负责受限解包、审批与发布事务，`registry` 负责稳定记录和原子写入，`gateway` 负责完整候选验证和重载，`lifecycle` 负责启停、恢复、备份，`control` 负责认证与持久任务。

注册表 `config/apps.yaml` 使用 JSON 表示，JSON 为 YAML 的子集。唯一写入口为受限运维进程。前端使用裁剪后的公开清单和独立静态快照。状态始终查询 Docker 与 HTTP 探测结果。

生产目录如下。

```text
/srv/pagehub/
  releases/<version>-<timestamp>/
  current -> releases/...
  config/pagehub.env
  config/apps.yaml
  gateway/Caddyfile
  gateway/public/catalog.json
  apps/<id>/releases/<archive-sha256>/
  secrets/admin.token
  state/manager/jobs/
  state/imports/
  state/failures/
  backups/<id>/<timestamp>/
```

应用 Compose 工程名固定 `pagehub-<id>`。版本目录独立，具名卷归属于稳定工程名。镜像在首次准备后固定为本地实际镜像 ID，恢复时禁用拉取和构建。

每个平台 release 的 runtime.env 保存该版本实际 Gateway/Portal 镜像 ID。升级构建可以复用构建标签，服务恢复及回滚使用 release 内保存的固定 ID。
