# 离线部署

普通源码发布包通过联网工具链构建，依赖 npm、Go 和镜像仓库。

完整离线产物需要在目标架构的联网 Linux 构建机执行 `scripts/build.sh`，准备平台镜像与全部启用应用的镜像，再执行 `scripts/package-release.sh offline`。打包程序收集已登记应用冻结运行配置中的镜像、平台镜像、目标架构二进制与前端产物，生成 SHA256SUMS。

```bash
sudo bash scripts/build.sh
sudo bash scripts/package-release.sh offline
```

离线目标需预装 Docker、Compose、systemd 和基础命令。解压后加载包附带参数再执行原始安装入口。

```bash
set -a
source offline.env
set +a
sudo -E bash install.sh
```

离线构建分支先核对 SHA256SUMS，再执行 docker load，并安装原始镜像引用到实际镜像 ID 的映射。导入应用以本地已准备镜像为前提，构建型 Compose 必须预先在构建机导入，启用的每个内置包须与构建机登记摘要一致。离线安装通过该映射复用已保存的构建镜像。

当前自动 Release 生成源码包及目标架构二进制包。完整离线包由受控构建机按实际应用清单生成，仍须在对应架构无外网环境验收。
