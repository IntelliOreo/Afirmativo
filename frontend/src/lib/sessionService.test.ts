import { beforeEach, describe, expect, it, vi } from "vitest";
import { checkSessionAccess, verifySession } from "./sessionService";

const apiMock = vi.fn();

vi.mock("@/lib/api", () => ({
  api: (...args: unknown[]) => apiMock(...args),
}));

describe("verifySession", () => {
  beforeEach(() => {
    apiMock.mockReset();
  });

  it("returns interviewStartedAt on success", async () => {
    apiMock.mockResolvedValue({
      ok: true,
      status: 200,
      data: {
        session: {
          interview_started_at: "2026-03-07T12:00:00.000Z",
        },
      },
    });

    await expect(verifySession("AP-123", "1234")).resolves.toEqual({
      ok: true,
      interviewStartedAt: "2026-03-07T12:00:00.000Z",
    });
    expect(apiMock).toHaveBeenCalledWith("/api/session/verify", {
      method: "POST",
      body: { session_code: "AP-123", pin: "1234" },
    });
  });

  it("maps status codes and network failures to typed reasons", async () => {
    apiMock.mockResolvedValueOnce({ ok: false, status: 404, data: null });
    apiMock.mockResolvedValueOnce({ ok: false, status: 410, data: null });
    apiMock.mockResolvedValueOnce({ ok: false, status: 429, data: null });
    apiMock.mockResolvedValueOnce({ ok: false, status: 500, data: null });
    apiMock.mockResolvedValueOnce({ ok: false, status: 401, data: null });
    apiMock.mockRejectedValueOnce(new Error("offline"));

    await expect(verifySession("AP-123", "1234")).resolves.toEqual({
      ok: false,
      reason: "not_found",
    });
    await expect(verifySession("AP-123", "1234")).resolves.toEqual({
      ok: false,
      reason: "expired",
    });
    await expect(verifySession("AP-123", "1234")).resolves.toEqual({
      ok: false,
      reason: "rate_limited",
    });
    await expect(verifySession("AP-123", "1234")).resolves.toEqual({
      ok: false,
      reason: "server",
    });
    await expect(verifySession("AP-123", "1234")).resolves.toEqual({
      ok: false,
      reason: "invalid_pin",
    });
    await expect(verifySession("AP-123", "1234")).resolves.toEqual({
      ok: false,
      reason: "network",
    });
  });
});

describe("checkSessionAccess", () => {
  beforeEach(() => {
    apiMock.mockReset();
  });

  it("maps access check outcomes to typed results", async () => {
    apiMock.mockResolvedValueOnce({ ok: true, status: 204, data: null });
    apiMock.mockResolvedValueOnce({ ok: false, status: 401, data: null });
    apiMock.mockResolvedValueOnce({ ok: false, status: 500, data: null });
    apiMock.mockRejectedValueOnce(new Error("offline"));

    await expect(checkSessionAccess("AP-123")).resolves.toEqual({ ok: true });
    expect(apiMock).toHaveBeenCalledWith("/api/session/access?session_code=AP-123");
    await expect(checkSessionAccess("AP-123")).resolves.toEqual({
      ok: false,
      reason: "unauthorized",
    });
    await expect(checkSessionAccess("AP-123")).resolves.toEqual({
      ok: false,
      reason: "unknown",
    });
    await expect(checkSessionAccess("AP-123")).resolves.toEqual({
      ok: false,
      reason: "network",
    });
  });
});
