import type { Plugin } from "vite";

// 仅由 dev:ui 加载，数据保存在当前开发进程内。
export function uiFixtures(): Plugin {
  const apps = [
    { id: "docs", name: "项目文档", description: "本地 UI 开发示例", path: "/docs/", kind: "static", startup: "always", state: "healthy", actions: [] },
    { id: "demo", name: "演示应用", description: "支持启停操作的演示数据", path: "/demo/", kind: "compose", startup: "manual", state: "stopped", actions: ["start", "stop", "restart"] },
    { id: "external", name: "外部服务", description: "已登记的独立服务", path: "/external/", kind: "external", startup: "manual", state: "unhealthy", actions: [] },
  ];
  const jobs = new Map<string, object>();
  return {
    name: "pagehub-ui-fixtures",
    apply: "serve",
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const request = req as { url?: string; method?: string };
        const url = request.url?.split("?")[0] || "";
        if (!/^\/(?:admin\/api|_pagehub\/api)\//.test(url) && url !== "/_pagehub/catalog.json") return next();
        let body: unknown;
        let status = 200;
        const match = url.match(/\/apps\/([^/]+)\/(status|start|stop|restart)$/);
        if (url.endsWith("/apps") || url.endsWith("catalog.json")) body = apps;
        else if (match) {
          const app = apps.find((item) => item.id === match[1]);
          if (!app) { status = 404; body = { error: "应用不存在" }; }
          else if (match[2] === "status") body = { state: app.state };
          else if (request.method === "POST" && app.actions.includes(match[2])) {
            app.state = match[2] === "stop" ? "stopped" : "healthy";
            const id = String(Date.now());
            body = { id, app_id: app.id, action: match[2], state: "succeeded" };
            jobs.set(id, body as object);
          } else { status = 403; body = { error: "操作未授权" }; }
        } else if (url.includes("/jobs/")) body = jobs.get(url.split("/").pop() || "");
        if (!body) { status = 404; body = { error: "演示接口不存在" }; }
        res.writeHead(status, { "Content-Type": "application/json; charset=utf-8" });
        res.end(JSON.stringify(body));
      });
    },
  };
}
