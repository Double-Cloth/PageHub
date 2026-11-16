import type { Action, AppEntry, AppState, Job } from "../models/app";

export async function request<T>(
  path: string,
  token = "",
  method = "GET",
  signal?: AbortSignal,
): Promise<T> {
  const response = await fetch(path, {
    method,
    signal: signal ?? AbortSignal.timeout(25_000),
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    cache: "no-store",
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as {
      error?: string;
    };
    throw new Error(body.error || `请求失败（${response.status}）`);
  }
  return response.json() as Promise<T>;
}

export function loadApps(token: string): Promise<AppEntry[]> {
  return request<AppEntry[]>(
    token ? "/admin/api/apps" : "/_pagehub/api/apps",
    token,
  );
}

export function loadStatus(
  id: string,
  token: string,
): Promise<{ state: AppState }> {
  const base = token ? "/admin/api" : "/_pagehub/api";
  return request(`${base}/apps/${encodeURIComponent(id)}/status`, token);
}

export function runAction(
  id: string,
  action: Action,
  token: string,
): Promise<Job> {
  return request(
    `/admin/api/apps/${encodeURIComponent(id)}/${action}`,
    token,
    "POST",
  );
}
