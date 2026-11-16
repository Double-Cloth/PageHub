# 应用适配器

`adapters.Adapter` 分离 Validate、Plan、Inspect、Start、Stop、Probe。Builtin 根据清单类型选择 Static、Image、Compose 实现路径，共享统一权限验证和生命周期行为。

## 导入事务

解包至系统临时目录，限制压缩包 64 MiB、展开 256 MiB、10000 项、20 层和 200 倍压缩比。拒绝绝对路径、路径逃逸、大小写冲突、重复项、链接、特殊文件。Go `os.Root` 限定解包边界。读取严格清单并核验登记冲突。

dry-run 仅在系统临时目录执行解析和 Compose config，打印清单、摘要、工程名与副作用。正式导入核对精确归档摘要，再获取应用锁，放入稳定候选目录，生成与校验冻结 Compose，拉取或构建镜像并固定实际镜像 ID。

共享路由提交区持有独立锁，重新读取注册表，验证完整候选 Caddy 配置，再启动常驻应用、重载网关、等待公开应用真实健康并原子提交注册表。失败时恢复先前路由和登记记录，首次导入创建的容器通过不带 `-v` 的 down 清理。失败详情写入 `state/failures/`，候选目录可供本机检查。

## Compose 权限

原始配置按字段白名单检查，再以固定参数调用 `docker compose config --format json` 检查合并结果。拒绝宿主端口、host network/PID/IPC、Docker Socket、privileged、cap_add、devices、自定义 entrypoint、生命周期 hooks、外部卷/网络、越界或可写 bind mount、远程构建上下文。

支持包内 Dockerfile 构建。应用代码和 Dockerfile 仍须经过管理员来源审核，生产宿主机执行权限来自包级审批。

## 已有应用接管

已有应用的接管需要先备份并准备经过审核的包，清单声明相同服务和实际镜像，保留原 Compose 项目名及具名卷名。

```bash
sudo bash pagehub.sh adopt-app /srv/uploads/existing.zip --project-name existing-project --dry-run
sudo bash pagehub.sh adopt-app /srv/uploads/existing.zip --project-name existing-project
```

接管检查每个服务各有一个运行实例、镜像 ID 一致、卷名和挂载目标逐项一致，无宿主端口及提升权限。既有环境变量仅写入 root 可读的冻结配置，公开计划和清单不包含这些秘密。成功后原工程名与卷引用保持稳定，仅 Web 服务加入入口网络。失败时恢复接管前冻结配置，保留全部已有卷。

包含外部卷、任意宿主 bind mount、匿名卷、多副本、已停止服务或包内构建声明的工程返回具体诊断，须先制定受信任迁移方案。接管包可使用本地固定 `sha256:<镜像ID>`，确保复用实际已有镜像。
