import { api } from "@/lib/api";

export type VerifyResult =
  | { ok: true; interviewStartedAt?: string; coupon?: VerifiedCoupon }
  | { ok: false; reason: "not_found" | "expired" | "invalid_pin" | "rate_limited" | "server" | "network" | "unknown" };

export type VerifiedCoupon = {
  code: string;
  maxUses: number;
  currentUses: number;
};

export type SessionAccessResult =
  | { ok: true }
  | { ok: false; reason: "unauthorized" | "network" | "unknown" };

export async function verifySession(sessionCode: string, pin: string): Promise<VerifyResult> {
  try {
    const result = await api<{
      session?: { interview_started_at?: string };
      coupon?: { code?: string; max_uses?: number; current_uses?: number };
    }>("/api/session/verify", {
      method: "POST",
      body: { session_code: sessionCode, pin },
    });

    if (result.ok && result.data?.session) {
      const coupon = result.data.coupon;
      return {
        ok: true,
        interviewStartedAt: result.data.session.interview_started_at,
        coupon: coupon && typeof coupon.code === "string" &&
          typeof coupon.max_uses === "number" &&
          typeof coupon.current_uses === "number"
          ? {
              code: coupon.code,
              maxUses: coupon.max_uses,
              currentUses: coupon.current_uses,
            }
          : undefined,
      };
    }

    if (result.status === 404) {
      return { ok: false, reason: "not_found" };
    }
    if (result.status === 410) {
      return { ok: false, reason: "expired" };
    }
    if (result.status === 429) {
      return { ok: false, reason: "rate_limited" };
    }
    if (result.status >= 500) {
      return { ok: false, reason: "server" };
    }
    if (!result.ok) {
      return { ok: false, reason: "invalid_pin" };
    }

    return { ok: false, reason: "unknown" };
  } catch {
    return { ok: false, reason: "network" };
  }
}

export async function checkSessionAccess(sessionCode: string): Promise<SessionAccessResult> {
  try {
    const result = await api<null>(`/api/session/access?session_code=${encodeURIComponent(sessionCode)}`);

    if (result.status === 204) {
      return { ok: true };
    }
    if (result.status === 401) {
      return { ok: false, reason: "unauthorized" };
    }

    return { ok: false, reason: "unknown" };
  } catch {
    return { ok: false, reason: "network" };
  }
}
