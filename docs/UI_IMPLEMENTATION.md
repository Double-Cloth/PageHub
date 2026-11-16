# PageHub UI v1.1

## 入口与功能

首页包含全屏 PixiJS 方格、统一颜色的自动蛇、红色食物、本地头像、个人资料和固定 Dock。项目展开与归位、首页散开与收拢、弹幕运动由 GSAP 驱动。DOM 优先渲染，PixiJS 按需加载，初始化失败时使用静态网格。简单应用列表位于 Dock 的服务状态面板中。

项目空间使用 d3-zoom 平移和缩放。首屏最多六项，手机竖屏两列，桌面和手机横屏三列。其余项目按固定环形坐标分布在外围。搜索覆盖名称、描述、路径及关键词，支持定位、回到中心、浏览器后退及拖动后取消点击。

手动游戏仅在精细指针、可悬停且宽度至少 768px 的环境开放。方向键进入和转向，Shift 加上下方向键调速，普通字符或 Esc 退出。支持暂停、死亡、通关和重开。窗口改变导致棋盘重建时要求重开，切入后台时暂停。自动引擎以偶数列哈密顿回路保底，捷径保持蛇身回路顺序，达到成长上限后反色并逐步回收。

## 本地预览

```bash
npm --prefix frontend ci
npm --prefix frontend run dev:ui -- --port 15174
```

`dev:ui` 提供八个演示项目和进程内模拟接口。演示密码可使用任意非空值。生产构建不包含该中间件。真实后端联调使用 `npm --prefix frontend run dev`。

默认资料采用 Double Cloth、Build. Explore. Create. 与本地编织标志占位头像，管理员可以修改。Barlow 字体及 OFL 许可证位于 `frontend/public/assets/fonts/`，页面运行时不请求外部字体或图片。

## 管理与展示配置

头像两秒内点击五次可发现管理入口。公网展示 SSH 转发说明。本机管理页面为 `http://127.0.0.1:8091/admin/`，公网网关继续拒绝 `/admin` 路由。

```bash
sudo bash pagehub.sh admin set-password
ssh -L 8091:127.0.0.1:8091 user@server
```

密码以 Argon2id 哈希与随机盐保存。管理会话有效期三十分钟，Cookie 使用 HttpOnly 与 SameSite=Strict，TLS 下附加 Secure。回环 HTTP 支持 SSH 使用场景。请求同时核验管理 Host、Origin 和 CSRF。密码设置命令重启 Manager 以撤销旧会话。兼容 Token 可在登录框显式选择，凭据仅存在页面内存。

展示管理支持修改名称、签名、本地头像、项目标题、显示开关、拖动或上下移动排序、撤销、添加和移除条目。保存采用 revision 检查与原子更新，冲突返回 409。配置保存于 `state/portal/ui-settings.json`，上一版本另存。启停仍经过 Manager 与 Runner，任务完成后继续读取真实健康状态。“任务与审计”面板显示生命周期任务、展示修改和弹幕管理记录。

## 离线项目准入

项目空间与 Dock 服务计数仅纳入当前登记版本已通过离线审核的公开应用。尚无审核记录的应用仍可在本机管理列表查看。应用更新导致登记指纹变化后，原审核记录自动失效。

完成方案第十二章的目标系统冷安装、出口阻断、依赖打包和核心功能验证后，本机管理员导入验收记录。

```bash
sudo bash pagehub.sh offline fingerprint my-app
sudo bash pagehub.sh offline approve my-app /srv/reviews/my-app.json
```

验收记录的 fingerprint 须匹配上述命令输出，evidence 定位实际保存的验收报告。

```json
{
  "appId": "my-app",
  "fingerprint": "当前登记指纹",
  "target": "Ubuntu 24.04 amd64",
  "evidence": "/srv/reviews/my-app-report.json",
  "passed": true,
  "verifiedAt": "2026-11-16T00:00:00Z"
}
```

审核命令登记管理员提交的结果。目标系统离线基础环境包、逐项目报告和真实 VM 冷安装仍需按部署环境制作与执行。已有离线发布脚本继续要求目标机具备所需基础环境，UI 实现不能替代这些验收证据。

## 弹幕服务与运维

独立 `pagehub-social` 进程通过 Unix Socket 提供历史、发布和 SSE。SQLite WAL 数据库位于 `state/social/messages.db`，提交成功后广播。历史最多返回五十条，保留上限五千条或三十天。发布采用纯文本、Unicode 字符限制、会话十五秒冷却、客户端与全局配额、重复内容抑制和来源校验。重启后的冷却仍从数据库查询。

管理删除与暂停发布经过 Manager 认证后转发至独立管理 Socket。Social 使用独立用户，systemd 限制其网络及秘密、Runner 和应用目录访问。客户端断线后退避重连并重新读取历史，删除事件同步在线客户端。

```bash
sudo bash pagehub.sh logs social
sudo bash pagehub.sh backup portal
sudo bash pagehub.sh restore portal /srv/pagehub/backups/portal-<timestamp>
```

备份使用 SQLite 一致性快照，并包含展示配置与审核记录。恢复前校验摘要并备份当前状态，停止服务后替换数据库和展示配置。离线审核须与当前版本重新核对。Social 已接入构建、systemd 启动、健康检查、日志和离线包二进制检查。

## 检查

```bash
cd frontend
npm test
npm run build
cd ../backend
go test ./...
go vet ./...
```

自动化覆盖回路、自动蛇长期生存与反色回收、手动碰撞和胜利、项目坐标、密码会话及 CSRF、展示版本冲突、离线指纹失效、SQLite 持久化与发布限制。Linux 互斥测试在 Linux 执行。`tests/integration/ui_verify.py` 使用真实网关和管理端验证密码登录、配置冲突、弹幕冷却、禁言及审计。

浏览器已检查方案列出的十一种视口、桌面游戏、搜索外围项目、管理员编辑保存、触控平移与双指缩放，以及一百次连续进出项目空间。实际低端手机性能、独立 HTTPS 管理 Origin、目标 Linux 冷安装及整机重启仍需在对应环境验收。
