# PageHub

## 部署

```bash
sudo PAGEHUB_PUBLIC_ORIGIN=https://your-domain.example:63445 bash install.sh
```

```bash
sudo PAGEHUB_INSTALL_DOCKER=true bash install.sh
```

```bash
sudo bash /srv/pagehub/current/restart.sh
sudo bash /srv/pagehub/current/restart.sh --check
```

## 管理

```bash
ssh -L 8091:127.0.0.1:8091 user@server
sudo bash /srv/pagehub/current/pagehub.sh status
sudo bash /srv/pagehub/current/pagehub.sh doctor
sudo bash /srv/pagehub/current/pagehub.sh app start my-app
sudo bash /srv/pagehub/current/pagehub.sh app stop my-app
sudo bash /srv/pagehub/current/pagehub.sh app restart my-app
sudo bash /srv/pagehub/current/pagehub.sh logs gateway
sudo bash /srv/pagehub/current/pagehub.sh logs my-app
```

## 导入应用

```bash
sudo bash pagehub.sh compose-review /srv/existing compose.yaml existing
sudo bash pagehub.sh add-app /srv/uploads/my-app.zip --dry-run
sudo bash pagehub.sh add-app /srv/uploads/my-app.zip
```

```bash
sudo bash pagehub.sh add-app /srv/uploads/my-app.zip --approved-sha256 <审核后的64位摘要>
```

## 维护

```bash
sudo bash pagehub.sh backup my-app
sudo bash pagehub.sh backup verify my-app /srv/pagehub/backups/my-app/<timestamp>
sudo bash pagehub.sh restore my-app /srv/pagehub/backups/my-app/<timestamp> --dry-run
sudo bash pagehub.sh app stop my-app
sudo bash pagehub.sh restore my-app /srv/pagehub/backups/my-app/<timestamp>
sudo bash pagehub.sh app start my-app
sudo bash pagehub.sh update my-app /srv/uploads/my-app-v2.zip
sudo bash pagehub.sh rollback static-demo
sudo bash pagehub.sh remove-app my-app
sudo bash pagehub.sh gateway reload
sudo bash pagehub.sh platform-update /srv/uploads/pagehub-new
sudo bash pagehub.sh platform-rollback
```

## 本地开发与检查

```bash
cd frontend
npm ci
npm run dev:ui
npm run dev
npm test
npm run build
```

```bash
cd backend
go test ./...
go vet ./...
go run ./cmd/manager
```

## 发布打包

```bash
bash scripts/package-release.sh source
bash scripts/package-release.sh offline
```

## 离线打包

```bash
bash scripts/package-release.sh source
bash scripts/package-release.sh offline
sha256sum -c pagehub-0.1.0-offline-amd64.tar.gz.sha256
mkdir pagehub-install
tar -xzf pagehub-0.1.0-offline-amd64.tar.gz -C pagehub-install
sudo bash pagehub-install/install.sh
```
