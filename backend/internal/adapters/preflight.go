package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/docker"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

type PreflightCheck struct {
	Name    string `json:"name"`
	State   string `json:"state"`
	Message string `json:"message"`
}

// PreflightExternal 从实际网关检查连通性，所有结果随审批计划展示。
func (b Builtin) PreflightExternal(ctx context.Context, a registry.Record) []PreflightCheck {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	checks := []PreflightCheck{}
	add := func(name, state, message string) { checks = append(checks, PreflightCheck{name, state, message}) }
	if a.Manifest.Kind == "external-compose" {
		out, err := b.Docker.Run(ctx, a.Manifest.Source.ComposeDirectory, append(b.externalArgs(a), "ps", "--all", "--format", "json")...)
		items, decodeErr := docker.DecodeObjects(out)
		switch {
		case err != nil || decodeErr != nil:
			add("containers", "warning", "无法查询既有容器，请核对 Compose 目录、文件、原项目名和 Docker 权限")
		case len(items) == 0:
			add("containers", "warning", "原项目没有已有容器，请先使用原部署工具执行 docker compose up -d；PageHub start 仅启动已有容器")
		default:
			status := containerStatus(a.Manifest.ID, items)
			state := "ok"
			if status.State != "running" {
				state = "warning"
			}
			add("containers", state, fmt.Sprintf("发现 %d 个已有容器，汇总状态为 %s", len(items), status.State))
		}
	}
	if _, err := b.Docker.Run(ctx, "", "network", "inspect", "pagehub-ingress"); err != nil {
		add("network", "warning", "共享网关网络 pagehub-ingress 不存在或无法查询，请先恢复 PageHub 网关网络")
	} else {
		add("network", "ok", "共享网关网络 pagehub-ingress 存在；容器服务需连接该网络并配置唯一 DNS 别名")
	}
	out, err := b.Docker.Run(ctx, "", "ps", "--filter", "label=com.docker.compose.project=pagehub-gateway", "--filter", "status=running", "--format", "{{.ID}}")
	ids := strings.Fields(string(out))
	if err != nil || len(ids) != 1 {
		for _, name := range []string{"dns", "port", "subpath"} {
			add(name, "warning", "需要唯一运行中的 PageHub 网关容器才能验证，请启动网关后重新执行 add-app --dry-run")
		}
		return checks
	}
	networks, inspectErr := b.Docker.Run(ctx, "", "inspect", "--format", "{{json .NetworkSettings.Networks}}", ids[0])
	var attached map[string]json.RawMessage
	if inspectErr != nil || json.Unmarshal(networks, &attached) != nil {
		add("gateway_network", "warning", "无法查询网关的网络连接，请核对 Docker 权限")
	} else if _, ok := attached["pagehub-ingress"]; !ok {
		add("gateway_network", "warning", "网关容器未连接 pagehub-ingress，请恢复网关 Compose 网络配置并重新部署网关")
	} else {
		add("gateway_network", "ok", "网关容器已连接 pagehub-ingress")
	}
	u, _ := url.Parse(a.Manifest.Source.Upstream)
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	// 分别查询 IPv4 和 IPv6，避免单栈服务因另一类记录不存在而被误报。
	if host == "localhost" || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()) {
		add("dns", "warning", "upstream 使用回环地址，会指向网关容器自身；请使用服务 DNS 或网关可达的宿主机地址")
	} else if net.ParseIP(host) != nil {
		add("dns", "ok", "upstream 使用 IP 地址，无需服务 DNS 解析")
	} else if _, err := b.Docker.Run(ctx, "", "exec", ids[0], "sh", "-c", `nslookup -type=A "$1" || nslookup -type=AAAA "$1"`, "preflight", host); err != nil {
		add("dns", "warning", "网关无法解析 "+host+"，请检查服务是否连接 pagehub-ingress、DNS 别名及名称冲突")
	} else {
		add("dns", "ok", "网关可解析 "+host)
	}
	if _, err := b.Docker.Run(ctx, "", "exec", ids[0], "nc", "-z", "-w", "5", host, port); err != nil {
		add("port", "warning", "网关无法连接 "+net.JoinHostPort(host, port)+"，请使用容器内部监听端口，检查服务启动状态与监听地址 0.0.0.0")
	} else {
		add("port", "ok", "网关可连接 "+net.JoinHostPort(host, port))
	}
	path := externalEntryPath(a)
	// wget 输出限长，使用首个响应和 Location 检查上游跳转。
	out, err = b.Docker.Run(ctx, "", "exec", ids[0], "sh", "-c", `wget -S -O - -T 5 "$1" 2>&1 | head -c 65536`, "preflight", a.Manifest.Source.Upstream+path)
	check := checkExternalSubpath(a, string(out))
	if err != nil {
		check = PreflightCheck{"subpath", "warning", "网关 HTTP 预检失败，请核对协议、TLS 和入口路径"}
	}
	checks = append(checks, check)
	return checks
}

func externalEntryPath(a registry.Record) string {
	if a.Manifest.RouteMode == "preserve_prefix" {
		return a.Manifest.Path
	}
	return "/"
}

var rootReference = regexp.MustCompile(`(?i)(?:src|href|action)\s*=\s*["']?(/[^"'<>\s]*)`)
var responseStatus = regexp.MustCompile(`HTTP/\d(?:\.\d)?\s+([0-9]{3})`)
var responseLocation = regexp.MustCompile(`(?im)^\s*Location:\s*([^\r\n]+)`)

func checkExternalSubpath(a registry.Record, response string) PreflightCheck {
	check := PreflightCheck{"subpath", "warning", "HTTP 预检未获得成功响应，请核对 route_mode、入口路径、协议和应用 base URL"}
	status := responseStatus.FindStringSubmatch(response)
	if len(status) == 0 || status[1][0] != '2' && status[1][0] != '3' {
		return check
	}
	if location := responseLocation.FindStringSubmatch(response); len(location) > 1 {
		target, err := url.Parse(strings.TrimSpace(location[1]))
		if err != nil || target.IsAbs() || target.Host != "" || strings.HasPrefix(target.Path, "/") && !strings.HasPrefix(target.Path, a.Manifest.Path) {
			check.Message = "上游重定向可能离开应用子路径，请配置外部 base URL 为 " + a.Manifest.Path + " 并检查 Location"
			return check
		}
	}
	for _, match := range rootReference.FindAllStringSubmatch(response, -1) {
		if !strings.HasPrefix(match[1], "//") && !strings.HasPrefix(match[1], a.Manifest.Path) {
			check.Message = "页面引用了应用子路径之外的根路径资源 " + match[1] + "，请配置资源 base URL 为 " + a.Manifest.Path
			return check
		}
	}
	check.State = "ok"
	check.Message = "上游路径 " + externalEntryPath(a) + " 可响应，未发现明显子路径冲突；仍需在浏览器验证登录、动态资源和 WebSocket"
	return check
}
