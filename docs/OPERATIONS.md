# PageHub

单台 Linux 服务器上的轻量级应用门户与部署管理平台。Vue 3 静态主页、共享 Caddy 网关、Go Manager 与本机受限 Runner，支持 Static、Image、Compose 应用包。

主页当前采用临时样式，业务入口由登记清单生成。后续可以独立替换 `frontend/src/ui/` 与页面组件。

## 部署

目标环境需要 systemd、Docker Engine、Docker Compose v2 或更新版本、Python 3.11+、Bash、curl、flock、tar、ss，支持 Linux amd64/arm64。首次源码安装需要联网下载工具链、npm/Go 依赖和镜像，预留至少 2 GiB 空间。建议使用独立的 Linux 测试 VM 先验收。

```bash
sudo PAGEHUB_PUBLIC_ORIGIN=https://your-domain.example:63445 bash install.sh
```

默认安装至 `/srv/pagehub`，宿主机入口为 `127.0.0.1:8080`。将现有 SakuraFrp 隧道指向该地址。安装器保留已存在的配置、凭据、应用目录和数据卷。

如服务器尚无 Docker，可先按 Docker 官方文档安装。Ubuntu/Debian 也支持显式授权安装依赖。

```bash
sudo PAGEHUB_INSTALL_DOCKER=true bash install.sh
```

正常机器重启由 systemd 恢复。手动恢复与只读检查使用以下命令。

```bash
sudo bash /srv/pagehub/current/restart.sh
sudo bash /srv/pagehub/current/restart.sh --check
```

`restart.sh` 只使用本地已有产物和镜像。默认恢复 `always` 应用，保留 `manual` 应用的当前启停状态。开机时 `manual` 容器的 `restart: "no"` 保证其保持停止。

## 管理

管理端监听服务器 `127.0.0.1:8091`。通过 SSH 转发连接，在浏览器访问 `http://127.0.0.1:8091/admin/`，输入服务器 `/srv/pagehub/secrets/admin.token` 内的 Token。

```bash
ssh -L 8091:127.0.0.1:8091 user@server
sudo bash /srv/pagehub/current/pagehub.sh status
sudo bash /srv/pagehub/current/pagehub.sh doctor
sudo bash /srv/pagehub/current/pagehub.sh app start my-app
sudo bash /srv/pagehub/current/pagehub.sh app stop my-app
sudo bash /srv/pagehub/current/pagehub.sh app restart my-app
sudo bash /srv/pagehub/current/pagehub.sh logs gateway
sudo bash /srv/pagehub/current/pagehub.sh logs my-app
```

Manager 使用独立低权限用户运行，通过 Unix Socket 向 Runner 提交带登记版本指纹的有限动作。任务状态和审计记录保存在 `state/manager/jobs/`，进程重启后保留。公开网关阻断 `/admin`，前端 Token 仅保存在当前页面内存。

## 导入应用

参考 `apps/_template/` 和 `apps/examples/`。包根目录须有 `app.yaml`，只支持 `.zip` 与 `.tar.gz`，生产容器镜像固定 sha256。`compose` 可使用包内 Dockerfile 构建，构建上下文限制在包目录内。

```bash
sudo bash pagehub.sh add-app /srv/uploads/my-app.zip --dry-run
sudo bash pagehub.sh add-app /srv/uploads/my-app.zip
```

交互导入先显示完整计划并要求确认可信来源。CI/已授权运维可以传入审核过的精确归档摘要。

```bash
sudo bash pagehub.sh add-app /srv/uploads/my-app.zip --approved-sha256 <审核后的64位摘要>
```

将批准的包放入 `apps/bundled/`，在 `config/policies/bundled-apps.json` 显式登记 `id`、`file`、`sha256`、`enabled`。首次安装会导入启用的包，重复安装保留现有登记版本。

`private` 应用默认在共享网关返回 403，并从公开清单隐藏。访问私有应用需要另外配置可信入口。应用必须自行支持公开子路径和同源安全要求。

## 维护

```bash
sudo bash pagehub.sh backup my-app
sudo bash pagehub.sh update my-app /srv/uploads/my-app-v2.zip
sudo bash pagehub.sh rollback static-demo
sudo bash pagehub.sh remove-app my-app
sudo bash pagehub.sh gateway reload
sudo bash pagehub.sh platform-update /srv/uploads/pagehub-new
sudo bash pagehub.sh platform-rollback
```

静态应用支持版本更新与回退。卷备份会停止应用、导出项目具名卷、记录 SHA-256 收据并恢复原运行状态。数据库外部备份与恢复由应用运维流程负责。容器应用更新须提供维护收据，写明已验证的备份、旧部署摘要及有效期，参见 `docs/BACKUP_AND_RECOVERY.md`。卸载保留应用版本文件和数据卷。

## 本地开发与检查

Windows 可开发前端和 Go 逻辑，部署与生命周期执行在 Linux 上进行。

```bash
cd frontend
npm ci
npm run dev
npm test
npm run build
```

```bash
cd backend
go test ./...
go vet ./...
go run ./cmd/manager
```

前端开发代理默认指向 `127.0.0.1:8091`。实际应用入口和状态需要安装后的平台。服务不可用时，公开页面尝试 `/_pagehub/catalog.json` 静态快照。

Linux Docker 集成测试入口为 `tests/integration/run.sh`，CI 会从打好的源码发布包运行。它覆盖三类真实应用、私有数据库、任务 API、路由、WebSocket、Range、数据保留、静态更新回退和卷备份。详细启动命令见 `docs/DEVELOPMENT.md`。

## 发布与验收范围

```bash
bash scripts/package-release.sh source
bash scripts/package-release.sh offline
```

源码包需要联网构建。离线包需要先在目标架构构建二进制、前端和所有运行镜像，然后执行离线打包。完整性校验及安装参数见 `docs/OFFLINE_DEPLOYMENT.md`。

当前本地验证记录见 `docs/TESTING.md`。真实 Linux systemd 首次安装、物理/VM 重启、Docker daemon 重启、arm64 与断网安装必须在对应环境继续验收。

## 文档

- [架构与目录](docs/ARCHITECTURE.md)
- [应用格式](docs/APP_SPEC.md)
- [应用适配器](docs/APP_ADAPTER.md)
- [API](docs/API.md)
- [部署](docs/DEPLOYMENT.md)
- [开机恢复](docs/STARTUP_AND_REBOOT.md)
- [安全](docs/SECURITY.md)
- [备份恢复](docs/BACKUP_AND_RECOVERY.md)
- [排障](docs/TROUBLESHOOTING.md)

独立 UI 开发使用 `npm --prefix frontend run dev:ui`。独立项目接入详见 [外部接入](EXTERNAL_APPS.md)，自动恢复演练和正式恢复详见 [恢复流程](RESTORE.md)。当前卷升级及回滚会核对程序生成的 verification.json 和备份内容摘要。
