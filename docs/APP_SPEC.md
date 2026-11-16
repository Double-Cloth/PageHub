# 应用格式

根目录 `app.yaml` 是单应用 YAML 文档。字段与结构见 `config/schema/app.schema.json` 和 `apps/_template/app.yaml`。运行时 Go 解析器还校验保留路径、重复路径、前缀重叠、规范化文件路径、类型专属字段与权限。

必填字段为 `schema_version`、`id`、`name`、`version`、`kind`、`path`、`startup`、`visibility`、`route_mode`、`source`、`lifecycle`、`backup`。`health` 缺省使用 `/`、15 秒间隔、5 秒超时。当前状态按 API 请求和恢复检查即时探测，前端每 30 秒刷新。

Static 使用 `source.static_dir` 和可选 `fallback`，支持该应用路径内的 SPA 回退。静态应用采用 `always`、`strip_prefix`，容器动作列表为空。

Image 使用 `source.image` 和 `container_port`。镜像支持 registry@sha256 摘要或已审核的本地 sha256 镜像 ID。Compose 使用 `compose_file`、`web_service`、`container_port`，平台生成完整冻结配置，并仅为 Web 服务添加入口网络。镜像自身声明的 VOLUME 目录自动分配稳定具名卷，名称由服务和容器目录确定，避免重新创建容器时生成空匿名卷。

`startup=manual` 所有服务的 Docker restart policy 强制为 `no`，`always` 为 `unless-stopped`。`visibility=private` 默认在公网返回 403。公开路由支持 `strip_prefix` 或 `preserve_prefix`，容器应用应自行处理 Base Path。

包内禁止实际 `.env`、PEM/KEY 私钥、Git 元数据和平台生成的 `pagehub.*` 文件。示例环境文件使用 `.env.example`。当前安全策略只允许只读包内 bind mount 与明确具名卷，容器秘密需要独立受控流程。
