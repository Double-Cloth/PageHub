# 应用模板

修改 `app.yaml` 的 ID、名称和公开路径，检查应用自身的 Base Path、Cookie、重定向与资源地址。

在模板目录内执行 `zip -r ../my-app.zip .`，然后执行 `sudo bash pagehub.sh add-app ../my-app.zip --dry-run`。

导入前审核镜像和代码。生产镜像必须固定到真实 sha256 摘要。密码和生产数据通过独立受控流程准备，不能打进应用包。
