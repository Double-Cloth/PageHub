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

## UI 展示与社交接口

| 方法 | 路径 | 内容 |
| --- | --- | --- |
| GET | `/_pagehub/api/portal` | 公开资料与通过当前版本离线审核的展示条目 |
| GET | `/admin/api/portal` | 完整展示配置与 revision |
| PUT | `/admin/api/portal` | 校验登记引用、离线审核与 revision 后原子保存 |
| POST | `/admin/api/session` | 密码验证，返回 csrf 并设置 HttpOnly 会话 Cookie |
| POST | `/admin/api/session/logout` | 撤销当前会话 |
| GET | `/_pagehub/social/messages?before=<UTC时间>` | 最近五十条可见消息与 enabled 状态 |
| POST | `/_pagehub/social/messages` | 提交纯文本 content，成功持久化后返回 201 |
| GET | `/_pagehub/social/events` | SSE 消息、删除、设置事件与心跳 |
| GET | `/_pagehub/social/health` | SQLite 健康检查 |
| POST | `/admin/api/social/messages/{id}/delete` | 认证后转发至 Social 管理 Socket |
| PUT | `/admin/api/social/settings` | 更新 enabled 并广播 |

密码会话同时要求 `X-CSRF-Token`，包括认证读取。管理 Host 限于 localhost 或回环地址，来源和跨站检查继续生效。兼容 Token 运维接口保留。公共应用数据新增 offlineReady，当前登记指纹须匹配本机审核记录。公共展示接口移除隐藏、私有和未通过离线审核的引用。

配置 PUT 的 revision 过期返回 409，字段无效返回 400。弹幕发布要求精确匹配配置的公开 Origin 与 JSON Content-Type。长度为 1–100 个 Unicode 码点，会话冷却十五秒，限流返回 429 和 Retry-After。真实客户端地址只接受网关覆盖的内部头。Social 无权访问 Runner 或管理凭据。

实施与运维步骤见 [UI 实施说明](UI_IMPLEMENTATION.md)。
