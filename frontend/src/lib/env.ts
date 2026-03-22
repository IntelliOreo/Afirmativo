// Local-only proxy target used by next dev and local container validation.
// In GCP, the load balancer owns /api routing before requests reach Next.
export function backendBaseURL(): string | null {
  const value = (process.env.API_PROXY_TARGET ?? "").trim().replace(/\/+$/, "");
  return value || null;
}

// Admin tooling is only enabled in local development.
// In development, ENABLE_ADMIN_TOOLS can explicitly disable/enable it.
// If APP_ENV is set, it must also be "development".
export function isAdminToolsEnabled(): boolean {
  if (process.env.NODE_ENV !== "development") return false;
  const enabled = process.env.ENABLE_ADMIN_TOOLS?.toLowerCase();
  if (enabled !== undefined) return enabled === "true";
  const appEnv = process.env.APP_ENV?.toLowerCase();
  if (appEnv !== undefined && appEnv !== "development") return false;
  return true;
}
