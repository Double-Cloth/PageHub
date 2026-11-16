# API

所有响应使用 JSON。公开 API 经 Caddy 转发至 Manager 的独立只读 Socket。Manager 不可用时前端读取静态快照。

| 方法 | 路径 | 内容 |
| --- | --- | --- |
| GET | `/_pagehub/api/health` | Manager 可用性 |
| GET | `/_pagehub/api/apps` | 公开应用的 id/name/description/path/kind/startup/state/actions |
| GET | `/_pagehub/api/apps/{id}/status` | 公开应用的真实粗粒度状态 |
| GET | `/_pagehub/catalog.json` | 最新成功发布的公开静态清单，状态 unknown |
| GET | `/admin/api/apps` | 本机管理员应用清单 |
| GET | `/admin/api/apps/{id}/status` | 本机管理员状态 |
| POST | `/admin/api/apps/{id}/start` | 创建启动任务 |
| POST | `/admin/api/apps/{id}/stop` | 创建停止任务 |
| POST | `/admin/api/apps/{id}/restart` | 创建重启任务 |
| GET | `/admin/api/jobs/{jobId}` | 查询任务 |
| GET | `/admin/api/audit` | 最近 200 条任务审计 |

管理端要求 `Authorization: Bearer <token>`。Token 从本机受控文件读取，常量时间比较，并检查 Origin 与 Sec-Fetch-Site。未认证 401，来源或动作权限不足 403，未知目标 404，并发变更 409。任务提交返回 202，客户端轮询 queued/running/succeeded/error。Runner 还会核验登记指纹、应用目录、当前清单和跨进程独占锁。

请求不接收任意 Shell、Docker 参数或文件路径。Runner Socket 仅提供 `GET /status/{id}` 与 `POST /execute`。任务完成表示 Compose 操作完成，健康状态由独立状态接口确认。
