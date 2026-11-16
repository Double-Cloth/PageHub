# 验证记录

验证日期为 2026-11-16。开发主机为 Windows amd64，使用 Docker Desktop 的 Linux Engine。Go 主机版本 1.27.1，Linux 测试版本 1.25.4，Docker Engine 29.9.2，Compose 5.6.1。前端使用锁文件中的 Vue、TypeScript 与 Vite 依赖。

## 已执行

| 检查 | 结果 |
| --- | --- |
| Windows 与 Linux `go test ./...`、`go vet ./...` | 通过 |
| Linux `go test -race ./...` | 通过 |
| Linux 跨进程锁、Docker CLI 子进程超时终止 | 通过 |
| 前端 `npm test`、TypeScript 检查、生产构建 | 通过 |
| Bash 全部入口与辅助脚本语法 | 通过 |
| systemd 三个单元 `systemd-analyze verify` | 通过，使用 Debian systemd 校验环境 |
| amd64、arm64 后端交叉编译 | 通过 |
| Static、Image、Compose 真实导入 | 通过 |
| dry-run、重复 ID、错误审批摘要、恶意归档与危险 Compose | 通过 |
| 公开清单隐藏私有应用、实际私有路由 403 | 通过 |
| localhost 管理鉴权、跨 Origin 拒绝、任务提交与查询 | 通过 |
| manual 默认恢复跳过、Docker restart policy、幂等恢复 | 通过 |
| Compose Web 与私有 Redis 网络隔离 | 通过 |
| WebSocket 双向数据、Range 206、静态资源、SPA、前缀模式 | 通过 |
| 数据经过启停与恢复保持一致 | 通过 |
| 镜像 VOLUME 自动具名化，容器移除重建后数据保留 | 通过 |
| 单应用健康失败、网关验证失败回退 | 通过，其他应用与登记记录保持可用 |
| 静态更新回退、容器更新与维护收据授权回滚 | 通过 |
| 卷备份、收据校验、原服务恢复 | 通过 |
| 既有 Compose 工程接管 | 通过，原项目名、卷、数据和环境变量保留 |
| 内置包启用清单与重复执行 | 通过，禁用条目跳过 |
| 退出 Manager/Runner 后主页和已运行应用 | 通过 |
| 解压源码发布包后的真实容器验收 | 通过 |
| 离线包生成、SHA256SUMS、docker load、镜像映射 | 通过 |
| 平台 release 内实际 Gateway/Portal 镜像 ID 选择 | 通过 |

容器集成使用独立测试工程和虚拟数据。自动入口为 `tests/integration/run.sh`，详细断言保存在 `tests/integration/verify.py`。CI 会打包、解压，再以发布包执行同一套容器验收。

## 浏览器验证

本次会话的 Browser 插件不可用，使用 Playwright。公开页面同时通过 Windows Edge 和 Linux Chromium 检查，管理页面在隔离浏览器容器内连接回环 Manager 接口。主验证视口为 1440×1000 和 390×844，截图使用 Asia/Shanghai 时区。

| 检查 | 结果 |
| --- | --- |
| URL、标题、主要内容、框架错误覆盖层 | 通过 |
| 正常公开页面控制台 | 0 条错误、0 条警告 |
| 页面 JavaScript 异常 | 0 条 |
| 搜索、无匹配状态、类型筛选、打开独立应用 | 通过 |
| 静态快照兜底与 unknown 状态 | 通过 |
| 错误 Token、管理员连接、退出、内存凭据 | 通过 |
| 管理页面启动、任务完成、真实健康状态、停止确认 | 通过 |
| 并发冲突错误反馈持续显示 | 通过 |
| 桌面与移动端横向溢出 | 通过 |

故障注入测试包含预期的 HTTP 401、409、503。页面脚本正常处理错误反馈，任务成功状态来自真实 Manager 与 Runner。检查过标题层级、正文与控制字体、白灰背景和蓝色操作色、列表间距、按钮及状态对齐、移动端换行。截图保存在本次任务的临时验证目录。

主页按需求使用临时样式。后续提供正式 UI 后，可以继续复用现有 API、数据模型、应用列表与操作逻辑。

## 目标环境验收

以下验收需要独立 Linux systemd VM 或测试服务器。

- 首次安装、重复安装和完整平台升级/回滚的 systemd 实机演练。
- 真实整机 reboot 后的自动恢复和数据核验。
- Docker daemon 重启后的恢复。
- 完全撤销外网条件下的首次离线安装与重启。
- arm64 实机运行。

整机测试入口为 `tests/reboot/verify.sh`，会比较实际 boot_id、卷名与数据摘要。运行结果应追加到本文件后再确认对应环境的发布验收。
