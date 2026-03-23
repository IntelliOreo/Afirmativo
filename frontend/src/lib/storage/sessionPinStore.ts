// Session PIN storage is a one-time handoff for UX only.
// Auth decisions must still come from the backend auth cookie.
function sessionPinKey(sessionCode: string): string {
  return `pin_${sessionCode}`;
}

function couponRevealKey(sessionCode: string): string {
  return `coupon_${sessionCode}`;
}

export type CouponReveal = {
  code: string;
  maxUses: number;
  currentUses: number;
};

export function writePin(sessionCode: string, pin: string): void {
  if (typeof window === "undefined") return;
  sessionStorage.setItem(sessionPinKey(sessionCode), pin);
}

export function readAndConsumePin(sessionCode: string): string | null {
  if (typeof window === "undefined") return null;

  const key = sessionPinKey(sessionCode);
  const raw = sessionStorage.getItem(key);
  sessionStorage.removeItem(key);

  if (raw == null) return null;
  const trimmed = raw.trim();
  return trimmed.length > 0 ? trimmed : null;
}

export function writeCouponReveal(sessionCode: string, coupon: CouponReveal): void {
  if (typeof window === "undefined") return;
  sessionStorage.setItem(couponRevealKey(sessionCode), JSON.stringify(coupon));
}

export function readAndConsumeCouponReveal(sessionCode: string): CouponReveal | null {
  if (typeof window === "undefined") return null;

  const key = couponRevealKey(sessionCode);
  const raw = sessionStorage.getItem(key);
  sessionStorage.removeItem(key);

  if (raw == null) return null;

  try {
    const parsed = JSON.parse(raw) as Partial<CouponReveal>;
    if (typeof parsed.code !== "string") return null;
    if (typeof parsed.maxUses !== "number" || typeof parsed.currentUses !== "number") return null;

    const code = parsed.code.trim();
    if (code.length === 0) return null;
    const maxUses = parsed.maxUses;
    const currentUses = parsed.currentUses;

    return {
      code,
      maxUses,
      currentUses,
    };
  } catch {
    return null;
  }
}
