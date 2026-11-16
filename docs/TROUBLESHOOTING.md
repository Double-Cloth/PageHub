# 排障

首先执行 `sudo bash pagehub.sh doctor` 和 `sudo bash restart.sh --check`。检查 Docker 是否运行、固定镜像是否在本地、pagehub-ingress 是否存在、网关端口是否被其他程序占用。

主页可见而应用 503 时，查看应用真实状态与日志，确认手动应用已启动、Web 服务监听正确端口、应用兼容公开子路径。应用停止时其路由保留，访问返回维护态 503。

Manager 不可用时，主页使用静态快照并显示状态未知。检查 pagehub-manager 与 pagehub-runner 的 systemd 日志和 `/run/pagehub*/` 权限。已运行的网站保持独立。

导入失败查看 `state/failures/`。正式路由与登记记录会尝试恢复，候选目录保留在 apps/id/releases/digest。重新导入同摘要前确认该候选未被注册表引用，再由管理员清理该失败候选目录，保留卷和诊断记录。

恢复发现缺镜像时，从经过审核的本地镜像归档 docker load，或在维护窗口重新准备指定版本。恢复入口会报告缺失并继续处理其他应用。

本机入口正常但公网不可达时，单独检查 SakuraFrp 客户端服务、隧道目标地址和实际公网端口。
