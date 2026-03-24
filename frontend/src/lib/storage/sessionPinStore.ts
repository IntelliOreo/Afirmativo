// Session PIN storage is a one-time handoff for UX only.
// Auth decisions must still come from the backend auth cookie.
function sessionPinKey(sessionCode: string): string {
  return `pin_${sessionCode}`;
}

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
