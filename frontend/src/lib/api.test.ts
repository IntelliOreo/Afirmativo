import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, ApiTimeoutError } from "./api";

vi.mock("./logger", () => ({
  log: {
    debug: vi.fn(),
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
  },
}));

const fetchMock = vi.fn();

function createAbortError(): Error {
  const error = new Error("The operation was aborted.");
  error.name = "AbortError";
  return error;
}

describe("api", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("throws ApiTimeoutError when fetch exceeds the configured timeout", async () => {
    fetchMock.mockImplementation((_url: string, init?: RequestInit) => new Promise((_, reject) => {
      init?.signal?.addEventListener("abort", () => {
        reject(createAbortError());
      }, { once: true });
    }));

    const request = api("/api/example", { timeoutMs: 100 });
    const expectation = expect(request).rejects.toMatchObject({
      name: "ApiTimeoutError",
      path: "/api/example",
      method: "GET",
      timeoutMs: 100,
    });

    await vi.advanceTimersByTimeAsync(100);

    await expectation;
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("retries once on a transient network failure when retries is enabled", async () => {
    fetchMock
      .mockRejectedValueOnce(new TypeError("network down"))
      .mockResolvedValueOnce(new Response(JSON.stringify({ ok: true }), {
        status: 200,
        headers: {
          "content-type": "application/json",
          "X-Request-Id": "req-123",
        },
      }));

    const request = api<{ ok: boolean }>("/api/example", { retries: 1 });

    await vi.runAllTimersAsync();

    await expect(request).resolves.toEqual({
      ok: true,
      status: 200,
      data: { ok: true },
      requestId: "req-123",
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("does not retry on HTTP error responses", async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({ error: "boom" }), {
      status: 500,
      headers: {
        "content-type": "application/json",
      },
    }));

    await expect(api<{ error: string }>("/api/example", { retries: 2 })).resolves.toEqual({
      ok: false,
      status: 500,
      data: { error: "boom" },
      requestId: "",
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
