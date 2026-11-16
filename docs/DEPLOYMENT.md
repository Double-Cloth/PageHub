# 部署

解压完整源码包到任意目录，使用 root 执行 `bash install.sh`。首次安装检查 Linux、架构、systemd、磁盘与端口，初始化固定目录，使用容器化 Go/Node 工具链构建，准备固定运行镜像，安装三个 systemd 单元，启动平台，按信任清单导入内置包并检查入口。

安装重复执行保留现有凭据和数据，进入恢复流程。中断后保留成功生成的配置与秘密，再次安装继续准备产物。升级使用 `pagehub.sh platform-update <源码目录>`，保留应用目录，先备份平台配置与凭据，保存旧 current 目标后切换版本。

配置位于 `/srv/pagehub/config/pagehub.env`，权限 600。首次执行可通过环境变量指定 PAGEHUB_ROOT、PAGEHUB_PUBLIC_ORIGIN、PAGEHUB_GATEWAY_PORT。安装根路径支持绝对路径中的字母、数字、下划线、横线和目录分隔符。

管理端默认 local，FRP 默认 external。SakuraFrp 客户端由其自身运维服务管理，将已有隧道 HTTP 目标设为 `127.0.0.1:8080`。公网 HTTPS 由隧道端处理。

源码与二进制发布工作流位于 `.github/workflows/`。跨架构构建产物需要分别在目标架构验收后发布。
