import type { Action, AppEntry, AppState, Job } from "../models/app";

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public retryAfter = 0,
  ) {
    super(message);
  }
}

export async function request<T>(
  path: string,
  token = "",
  method = "GET",
  signal?: AbortSignal,
  body?: unknown,
): Promise<T> {
  const response = await fetch(path, {
    method,
    signal: signal ?? AbortSignal.timeout(25_000),
    headers: {
      ...(token.startsWith("session:")
        ? { "X-CSRF-Token": token.slice(8) }
        : token
          ? { Authorization: `Bearer ${token}` }
          : {}),
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: "no-store",
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as {
      error?: string;
    };
    throw new ApiError(
      body.error || `请求失败（${response.status}）`,
      response.status,
      Number(response.headers.get("Retry-After")) || 0,
    );
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
