# 备份与恢复

`pagehub.sh backup <id>` 根据能力执行。静态应用输出可重新导入的 bundle.zip。容器应用要求 `backup.mode=volume`，停止该应用后导出其 Compose 工程标签下的具名卷，并在原先运行时恢复应用。每份成功备份包含文件 SHA-256 收据。`none` 和需要 external 业务备份的容器返回明确错误。

备份保存在 `/srv/pagehub/backups/<id>/<timestamp>/`，应另行复制到异机受控位置。卷备份属于停止写入后的文件系统快照，业务恢复仍需针对应用数据库版本验证。

## 容器应用更新

先完成备份和恢复演练，在独立受控 JSON 文件中登记如下维护收据，然后传给更新命令。

```json
{
  "id": "my-app",
  "digest": "旧登记记录中的64位归档摘要",
  "backup": "/srv/pagehub/backups/my-app/已验证备份/receipt.json",
  "restore_verified": true,
  "expires_at": "2030-01-01T00:00:00Z"
}
```

有效期应按实际维护窗口填写。真实旧摘要可从本机 `config/apps.yaml` 获取。

```bash
sudo bash pagehub.sh update my-app /srv/uploads/my-app-v2.zip --maintenance-receipt /srv/approved/maintenance.json
```

新版本准备失败保留旧实例，切换或健康失败恢复旧配置和旧路由。数据库不可逆迁移需要业务备份恢复。静态应用提供一键版本回退，容器版本回退需先确认当前数据兼容旧镜像，或完成已验证的业务备份恢复，再提供绑定当前版本摘要的维护收据。

```bash
sudo bash pagehub.sh rollback my-app --maintenance-receipt /srv/approved/rollback.json
```

## 手动卷恢复

停止应用并检查本机备份收据的全部 SHA-256，记录现有卷名，再对相同命名卷恢复。恢复会覆盖数据，必须先保留当前卷快照并使用独立维护窗口。不要执行带 `-v` 的 compose down。恢复完成后先验证数据库，再恢复 Web 服务并探测公开路径。

平台更新保存 config/secrets 快照和旧 release 路径。`platform-rollback` 切换旧 current 并重启控制平面与入口，应用数据保存在独立稳定目录。
