# 安全边界

共享路径属于同一 Origin。受信任应用代码可在该 Origin 执行，须按同源应用供应链审核。Cookie Path 与卡片隐藏只承担路由或展示职责。

公开 Portal、Caddy、Manager 均不挂载 Docker Socket。Runner 在本机 root 服务中通过 Docker CLI 执行有限登记动作，Socket 目录 0750、Socket 0660，Manager 使用专用 pagehub 用户组。公开只读 Socket 位于独立的 `/run/pagehub-public`，网关只能接触该目录。

管理端绑定 127.0.0.1，使用长随机 Token，拒绝跨 Origin 管理请求。公开网关阻断管理路径，公网用户没有上传入口。私有应用在共享网关固定返回 403。

应用包必须来源可信。管理员审批绑定整个归档 SHA-256，正式导入再次核对归档内容。受限解包和 Compose 检查减少路径、运行权限和项目边界风险。包中的应用代码、Dockerfile 与容器镜像须由维护者审核。

配置、秘密与备份使用受限权限，公开清单裁剪存储路径、镜像信息和秘密。CLI logs 为本机 root 排障入口，应用自身仍应避免向日志写入密码。替换管理员 Token 后新请求立即使用新凭据。
