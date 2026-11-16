# 恢复演练与正式恢复

```bash
sudo bash pagehub.sh backup verify my-app /srv/pagehub/backups/my-app/<timestamp>
sudo bash pagehub.sh restore my-app /srv/pagehub/backups/my-app/<timestamp> --dry-run
sudo bash pagehub.sh app stop my-app
sudo bash pagehub.sh restore my-app /srv/pagehub/backups/my-app/<timestamp>
sudo bash pagehub.sh app start my-app
```

verify 核对备份 SHA-256 及当前登记摘要。静态包解压至临时目录，具名卷归档解压至隔离临时卷，生成逐文件 SHA-256 后删除演练卷。链接、特殊文件和越界路径会被拒绝。演练验证文件系统恢复能力，数据库业务一致性需要应用检查。

--dry-run 执行相同演练，生成 verification.json 和 verified-files 清单。正式恢复重新演练，自动备份当前数据后写入，报告写入 state/restores，记录源备份、安全备份和结果。

卷恢复要求应用已停止，恢复后比较文件摘要，保持停止状态供数据库检查。部分恢复失败后使用报告中的 safety_backup 再次恢复。静态恢复解包到 `apps/<id>/restores/restore-<随机编号>/`，校验后切换网关并保存注册表中的 `static_restore`。原 Release 目录与版本摘要保持一致，旧登记默认使用 Release 内容。后续备份读取当前恢复内容，更新和回滚保留对应版本的恢复目录引用。网关提交或注册表保存失败时回退到原登记与路由，恢复目录保留供诊断。恢复限定为当前登记版本，跨版本迁移使用应用自身流程。外部模式继续使用原备份工具。

更新维护收据用于授权维护窗口，应引用实际验证产物。verification.json 保存程序执行的恢复演练结果。
