export type AppState =
  | "unknown"
  | "stopped"
  | "starting"
  | "healthy"
  | "unhealthy"
  | "stopping"
  | "error";
export type Action = "start" | "stop" | "restart";
export interface AppEntry {
  id: string;
  name: string;
  description?: string;
  path: string;
  kind: "static" | "image" | "compose" | "external" | "external-compose";
  startup: "always" | "manual";
  state: AppState;
  actions: Action[] | null;
}
export interface Job {
  id: string;
  app_id: string;
  action: Action;
  state: "queued" | "running" | "succeeded" | "error";
  error?: string;
}
export const stateLabels: Record<AppState, string> = {
  unknown: "状态未知",
  stopped: "已停止",
  starting: "启动中",
  healthy: "运行正常",
  unhealthy: "健康异常",
  stopping: "停止中",
  error: "发生错误",
};
export const actionLabels: Record<Action, string> = {
  start: "启动",
  stop: "停止",
  restart: "重启",
};
