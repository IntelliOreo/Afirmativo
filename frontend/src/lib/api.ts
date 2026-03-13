import { log } from "./logger";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "";
const DEFAULT_TIMEOUT_MS = 30_000;
const RETRY_BASE_DELAY_MS = 1_000;

interface ApiOptions {
  method?: string;
  body?: unknown;
  credentials?: RequestCredentials;
  timeoutMs?: number;
  retries?: number;
}

interface ApiResult<T> {
  ok: boolean;
  status: number;
  data: T | null;
  requestId: string;
}

export class ApiTimeoutError extends Error {
  constructor(
    public readonly path: string,
    public readonly method: string,
    public readonly timeoutMs: number,
  ) {
    super(`${method} ${path} timed out after ${timeoutMs}ms`);
    this.name = "ApiTimeoutError";
  }
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

function isAbortError(error: unknown): boolean {
  if (!error || typeof error !== "object" || !("name" in error)) return false;
  return error.name === "AbortError";
}

function isRetryableRequestError(error: unknown): boolean {
  return error instanceof ApiTimeoutError || error instanceof TypeError;
}

function retryDelayMs(attempt: number): number {
  return RETRY_BASE_DELAY_MS * (2 ** attempt);
}

function wait(ms: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

export async function api<T = unknown>(
  path: string,
  opts: ApiOptions = {},
): Promise<ApiResult<T>> {
  const method = (opts.method || "GET").toUpperCase();
  const url = `${API_URL}${path}`;
  const timeoutMs = opts.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  const retries = Math.max(0, opts.retries ?? 0);
  const headers = opts.body != null ? { "Content-Type": "application/json" } : undefined;
  const body = opts.body != null ? JSON.stringify(opts.body) : undefined;

  log.debug(`calling ${method} ${path}`, {
    has_body: opts.body != null,
    body_keys: bodyKeys(opts.body),
    body: allowSensitiveDebugLogs ? opts.body : sanitizeDebugValue(opts.body),
    sensitive_debug_logs_enabled: allowSensitiveDebugLogs,
  });

  for (let attempt = 0; attempt <= retries; attempt += 1) {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => {
      controller.abort();
    }, timeoutMs);
    const start = performance.now();

    try {
      const res = await fetch(url, {
        method,
        headers,
        body,
        credentials: opts.credentials ?? "include",
        signal: controller.signal,
      });

      const elapsed = Math.round(performance.now() - start);
      const requestId = res.headers.get("X-Request-Id") ?? "";

      let data: T | null = null;
      const contentType = res.headers.get("content-type");
      if (contentType?.includes("application/json")) {
        data = await res.json();
      }

      if (!res.ok) {
        log.warn(`${method} ${path} failed`, {
          status: res.status,
          elapsed_ms: elapsed,
          request_id: requestId || undefined,
        });
      } else {
        log.debug(`${method} ${path} done`, {
          status: res.status,
          elapsed_ms: elapsed,
          request_id: requestId || undefined,
        });
      }

      return { ok: res.ok, status: res.status, data, requestId };
    } catch (error) {
      const wrappedError = isAbortError(error)
        ? new ApiTimeoutError(path, method, timeoutMs)
        : error;

      if (wrappedError instanceof ApiTimeoutError) {
        log.warn(`${method} ${path} timed out`, {
          timeout_ms: timeoutMs,
          retry_attempts_remaining: retries - attempt,
        });
      }

      if (attempt >= retries || !isRetryableRequestError(wrappedError)) {
        throw wrappedError;
      }

      const delayMs = retryDelayMs(attempt);
      log.warn(`${method} ${path} retrying after transient failure`, {
        retry_attempt: attempt + 1,
        retry_in_ms: delayMs,
        reason: wrappedError instanceof ApiTimeoutError ? "timeout" : "network",
      });
      await wait(delayMs);
    } finally {
      clearTimeout(timeoutId);
    }
  }

  throw new Error(`Unreachable api retry loop for ${method} ${path}`);
}
