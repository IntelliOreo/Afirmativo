import { log } from "./logger";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "";

interface ApiOptions {
  method?: string;
  body?: unknown;
  credentials?: RequestCredentials;
}

interface ApiResult<T> {
  ok: boolean;
  status: number;
  data: T | null;
}

export async function api<T = unknown>(
  path: string,
  opts: ApiOptions = {},
): Promise<ApiResult<T>> {
  const method = opts.method || "GET";
  const url = `${API_URL}${path}`;

  log.debug(`calling ${method} ${path}`, opts.body != null ? { body: opts.body as Record<string, unknown> } : undefined);

  const start = performance.now();

  const res = await fetch(url, {
    method,
    headers: opts.body != null ? { "Content-Type": "application/json" } : undefined,
    body: opts.body != null ? JSON.stringify(opts.body) : undefined,
    credentials: opts.credentials ?? "include",
  });

  const elapsed = Math.round(performance.now() - start);

  let data: T | null = null;
  const contentType = res.headers.get("content-type");
  if (contentType?.includes("application/json")) {
    data = await res.json();
  }

  if (!res.ok) {
    log.warn(`${method} ${path} failed`, { status: res.status, elapsed_ms: elapsed, data: data as unknown as Record<string, unknown> });
  } else {
    log.debug(`${method} ${path} done`, { status: res.status, elapsed_ms: elapsed });
  }

  return { ok: res.ok, status: res.status, data };
}
