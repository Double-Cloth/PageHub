# 开发

前端使用 Vue 3、TypeScript、Vite。主页目录与管理员操作共用数据模型，临时视觉集中在 `src/ui/styles.css`。公开页面无管理登录入口，管理页面通过 SSH 端口转发访问 `/admin/`。

后端使用 Go 1.25+ 和 yaml.v3。任务与审计采用原子 JSON 文件持久化，注册表为兼容 YAML 的 JSON。Linux 使用 flock 跨进程互斥，Windows 可执行解析与权限单元测试，运维动作限定 Linux。

```bash
cd backend && go test -race ./... && go vet ./...
cd frontend && npm ci && npm test && npm run build
```

## 真实 Docker 集成测试

必须使用没有运行 PageHub 实例的独立 Docker Engine。测试主动拒绝现有 pagehub-ingress，保留其他 Docker 项目。提前构建 Linux 后端二进制到 bin/、构建前端到 frontend/dist/。

```bash
docker pull caddy:2.10.2-alpine
docker build -t pagehub-test-tools -f tests/integration/Dockerfile .
docker volume create pagehub-test-runtime
root=$(docker volume inspect pagehub-test-runtime --format '{{.Mountpoint}}')
docker run --rm --network host \
  -e PAGEHUB_ROOT="$root" -e PAGEHUB_GATEWAY_PORT=18080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$root:$root" -v /run/pagehub-public:/run/pagehub-public \
  -v "$PWD:/workspace:ro" \
  pagehub-test-tools /workspace/tests/integration/run.sh
```

测试 helper 使用本机 Docker Socket，在隔离测试环境内创建、启停和清理测试工程。生产组件遵循独立的受限 Socket 模型。测试卷默认保留以供诊断，重新运行使用新的空卷。

完整安装和真实 reboot 验收需要 systemd VM，不能由普通容器集成测试替代。

## 独立 UI 开发

运行 `npm --prefix frontend run dev:ui` 启动本地演示接口和 UI，无需 Docker 或后端。管理页面 /admin/ 接受任意非空演示 Token，状态保存在当前 Vite 进程中。默认仅监听 127.0.0.1。端口被占用时追加 `-- --port 15173`。生产构建不加载演示中间件。真实后端联调使用原 dev 命令。
