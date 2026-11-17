export type AppState =
  | "unknown"
  | "stopped"
  | "starting"
  | "healthy"
  | "unhealthy"
  | "stopping"
  | "error";
export type Action =
  | "start"
  | "stop"
  | "restart"
  | "backup"
  | "restore"
  | "update"
  | "update-self"
  | "rollback"
  | "uninstall";
export type MaintenanceAction =
  "backup" | "restore" | "update" | "update-self" | "rollback" | "uninstall";
export interface DeploymentPlan {
  sha256: string;
  changes: {
    id: string;
    action: string;
    current_commit?: string;
    target_commit?: string;
    project?: string;
    volume?: string;
    path: string;
  }[];
}
export interface AppEntry {
  id: string;
  name: string;
  description?: string;
  path: string;
  kind: "static" | "image" | "compose" | "external" | "external-compose";
  startup: "always" | "manual";
  state: AppState;
  actions: Action[] | null;
  offlineReady?: boolean;
  displayReady?: boolean;
  keywords?: string[];
  iconPath?: string;
  managed?: boolean;
  commit?: string;
  image?: string;
  driver?: string;
}
export interface Job {
  changes?: DeploymentPlan["changes"];
  commit?: string;
  id: string;
  app_id: string;
  action: Action;
  state: "queued" | "running" | "succeeded" | "error";
  error?: string;
  result?: Record<string, string>;
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
  backup: "备份",
  restore: "恢复",
  update: "更新",
  "update-self": "更新平台",
  rollback: "回滚",
  uninstall: "卸载",
};
