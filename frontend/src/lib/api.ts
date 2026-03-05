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

function bodyKeys(body: unknown): string[] | undefined {
  if (!body || typeof body !== "object" || Array.isArray(body)) return undefined;
  return Object.keys(body as Record<string, unknown>).slice(0, 12);
}

const SENSITIVE_KEY_RE = /(pin|token|authorization|cookie|secret|password|api[_-]?key|jwt|bearer|auth)/i;
const allowSensitiveDebugLogs =
  (process.env.NEXT_PUBLIC_ALLOW_SENSITIVE_DEBUG_LOGS ?? "").trim().toLowerCase() === "true";

function sanitizeDebugValue(value: unknown, seen: WeakSet<object> = new WeakSet()): unknown {
  if (value == null) return value;
  if (typeof value !== "object") return value;
  if (seen.has(value as object)) return "[Circular]";
  seen.add(value as object);

  if (Array.isArray(value)) {
    return value.map((item) => sanitizeDebugValue(item, seen));
  }

  const out: Record<string, unknown> = {};
  for (const [key, raw] of Object.entries(value as Record<string, unknown>)) {
    if (SENSITIVE_KEY_RE.test(key)) {
      out[key] = "[REDACTED]";
      continue;
    }
    out[key] = sanitizeDebugValue(raw, seen);
  }
  return out;
}

export async function api<T = unknown>(
  path: string,
  opts: ApiOptions = {},
): Promise<ApiResult<T>> {
  const method = opts.method || "GET";
  const url = `${API_URL}${path}`;

  log.debug(`calling ${method} ${path}`, {
    has_body: opts.body != null,
    body_keys: bodyKeys(opts.body),
    body: allowSensitiveDebugLogs ? opts.body : sanitizeDebugValue(opts.body),
    sensitive_debug_logs_enabled: allowSensitiveDebugLogs,
  });

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
    log.warn(`${method} ${path} failed`, { status: res.status, elapsed_ms: elapsed });
  } else {
    log.debug(`${method} ${path} done`, { status: res.status, elapsed_ms: elapsed });
  }

  return { ok: res.ok, status: res.status, data };
}
