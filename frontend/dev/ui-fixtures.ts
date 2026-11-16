import type { Plugin } from "vite";
import { defaultPresentation } from "../src/models/presentation";

// 仅 dev:ui 使用，模拟接口不会进入生产构建。
export function uiFixtures(): Plugin {
  const apps = [
    {
      id: "docs",
      name: "项目文档",
      description: "想法、过程与使用指南",
      path: "/docs/",
      kind: "static",
      startup: "always",
      state: "healthy",
      actions: [],
      offlineReady: true,
    },
    {
      id: "studio",
      name: "创作工作台",
      description: "让下一个想法发生",
      path: "/studio/",
      kind: "compose",
      startup: "always",
      state: "healthy",
      actions: ["start", "stop", "restart"],
      offlineReady: true,
    },
    {
      id: "gallery",
      name: "影像档案",
      description: "日常片段的私人收藏",
      path: "/gallery/",
      kind: "static",
      startup: "always",
      state: "healthy",
      actions: [],
      offlineReady: true,
    },
    {
      id: "notes",
      name: "随手记",
      description: "留住转瞬即逝的灵感",
      path: "/notes/",
      kind: "image",
      startup: "manual",
      state: "stopped",
      actions: ["start", "stop", "restart"],
      offlineReady: true,
    },
    {
      id: "tools",
      name: "实用工具",
      description: "为小事做一些小工具",
      path: "/tools/",
      kind: "static",
      startup: "always",
      state: "healthy",
      actions: [],
      offlineReady: true,
    },
    {
      id: "lab",
      name: "实验室",
      description: "尚在生长的有趣尝试",
      path: "/lab/",
      kind: "compose",
      startup: "manual",
      state: "unknown",
      actions: ["start", "stop", "restart"],
      offlineReady: true,
    },
    {
      id: "archive",
      name: "旧作仓库",
      description: "回头看看走过的路",
      path: "/archive/",
      kind: "static",
      startup: "always",
      state: "healthy",
      actions: [],
      offlineReady: true,
    },
    {
      id: "read",
      name: "阅读清单",
      description: "值得花时间的文字",
      path: "/read/",
      kind: "image",
      startup: "manual",
      state: "unhealthy",
      actions: ["start", "stop", "restart"],
      offlineReady: true,
    },
  ];
  let settings = structuredClone(defaultPresentation);
  settings.revision = 1;
  settings.projects = apps.map((app, index) => ({
    id: app.id,
    appId: app.id,
    displayOrder: index,
    displayEnabled: true,
    titleOverride: "",
    keywords: [],
  }));
  const jobs = new Map<string, object>();
  const messages = [
    {
      id: "welcome",
      content: "你好，欢迎来到这个小小的数字空间。",
      created_at: new Date().toISOString(),
    },
  ];
  const streams = new Set<import("node:http").ServerResponse>();
  let lastSent = 0;
  let enabled = true;
  return {
    name: "pagehub-ui-fixtures",
    apply: "serve",
    configureServer(server) {
      server.httpServer?.on("close", () => {
        streams.forEach((stream) => stream.end());
        streams.clear();
      });
      server.middlewares.use(async (req, res, next) => {
        const url = req.url?.split("?")[0] || "";
        if (
          !/^\/(?:admin\/api|_pagehub\/api|_pagehub\/social)\//.test(url) &&
          url !== "/_pagehub/catalog.json"
        )
          return next();
        res.setHeader("Cache-Control", "no-store");
        if (url.endsWith("/events")) {
          res.writeHead(200, { "Content-Type": "text/event-stream" });
          res.write(": connected\n\n");
          streams.add(res);
          req.on("close", () => streams.delete(res));
          return;
        }
        let payload: any = {};
        if (req.method === "POST" || req.method === "PUT") {
          try {
            let raw = "";
            for await (const chunk of req) raw += chunk;
            payload = raw ? JSON.parse(raw) : {};
          } catch {
            res.writeHead(400);
            res.end();
            return;
          }
        }
        let body: unknown;
        let status = 200;
        const match = url.match(
          /\/apps\/([^/]+)\/(status|start|stop|restart)$/,
        );
        if (url.endsWith("/session")) body = { csrf: "demo-session" };
        else if (url.endsWith("/logout")) body = { ok: true };
        else if (url.endsWith("/portal")) {
          if (req.method === "PUT") {
            if (payload.revision !== settings.revision) {
              status = 409;
              body = { error: "展示配置已被修改，请重新加载" };
            } else {
              settings = { ...payload, revision: settings.revision + 1 };
              body = settings;
            }
          } else
            body = url.startsWith("/admin")
              ? settings
              : {
                  ...settings,
                  projects: settings.projects.filter(
                    (project) => project.displayEnabled,
                  ),
                };
        } else if (url.endsWith("/apps") || url.endsWith("catalog.json"))
          body = apps;
        else if (match) {
          const app = apps.find((item) => item.id === match[1]);
          if (!app) {
            status = 404;
            body = { error: "应用不存在" };
          } else if (match[2] === "status") body = { state: app.state };
          else if (req.method === "POST" && app.actions.includes(match[2]!)) {
            app.state = match[2] === "stop" ? "stopped" : "healthy";
            const id = String(Date.now());
            body = { id, app_id: app.id, action: match[2], state: "succeeded" };
            jobs.set(id, body as object);
          } else {
            status = 403;
            body = { error: "操作未授权" };
          }
        } else if (url.endsWith("/audit")) body = Array.from(jobs.values());
        else if (url.includes("/jobs/"))
          body = jobs.get(url.split("/").pop() || "");
        else if (url.endsWith("/messages") && req.method === "GET")
          body = { messages, enabled };
        else if (url.endsWith("/messages") && req.method === "POST") {
          if (
            !enabled ||
            Date.now() - lastSent < 15000 ||
            !payload.content ||
            Array.from(String(payload.content)).length > 100
          ) {
            status = 429;
            body = { error: "每 15 秒只能发送一条有效弹幕" };
          } else {
            lastSent = Date.now();
            const message = {
              id: String(Date.now()),
              content: String(payload.content),
              created_at: new Date().toISOString(),
            };
            messages.unshift(message);
            streams.forEach((stream) =>
              stream.write(
                `event: message\ndata: ${JSON.stringify(message)}\n\n`,
              ),
            );
            body = message;
          }
        } else if (url.endsWith("/settings")) {
          enabled = payload.enabled;
          streams.forEach((stream) =>
            stream.write(
              `event: settings\ndata: ${JSON.stringify({ enabled })}\n\n`,
            ),
          );
          body = { enabled };
        } else if (url.endsWith("/delete")) {
          const id = url.split("/").at(-2)!;
          const index = messages.findIndex((message) => message.id === id);
          if (index >= 0) messages.splice(index, 1);
          streams.forEach((stream) =>
            stream.write(`event: delete\ndata: ${id}\n\n`),
          );
          body = { ok: true };
        }
        if (!body) {
          status = 404;
          body = { error: "演示接口不存在" };
        }
        res.writeHead(status, {
          "Content-Type": "application/json; charset=utf-8",
        });
        res.end(JSON.stringify(body));
      });
    },
  };
}
