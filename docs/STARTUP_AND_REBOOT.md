# 开机恢复

systemd 依赖顺序为 Docker → Runner/Manager → pagehub.service。pagehub.service 调用 `restart.sh --boot`。访问平面的容器与控制平面进程独立存活。

恢复入口先确认已有网络和镜像，执行 `compose up -d --no-build --pull never` 恢复 Portal 与网关，再逐个恢复已登记 always 应用。manual 服务使用 `restart: "no"` 并跳过恢复。单应用错误累计报告，其他应用继续处理。

```bash
sudo systemctl status pagehub.service pagehub-runner.service pagehub-manager.service
sudo journalctl -u pagehub.service -b --no-pager
sudo bash /srv/pagehub/current/restart.sh --check
sudo bash /srv/pagehub/current/restart.sh
```

默认恢复保留正在运行的 manual 应用。Docker/整机重启后它们保持停止。`--force` 需要在交互终端确认已完成安全写入停止窗口，然后逐个停止并启动 always 应用。

整机专项验收需在独立 VM 执行真实 reboot，重连后检查各容器真实状态和数据库内容，随后重复 Docker daemon 重启、撤销外网路由、重复恢复与端口冲突测试。测试记录中必须注明镜像 ID、卷名和写入数据摘要。

已安装 Compose 示例夹具的隔离 VM 可以使用仓库内脚本保存与校验基线。verify 只检查恢复结果，boot_id 未发生变化时会拒绝给出整机重启通过结论。

```bash
sudo bash tests/reboot/verify.sh prepare
sudo reboot
# 重连后执行
sudo bash tests/reboot/verify.sh verify
```
