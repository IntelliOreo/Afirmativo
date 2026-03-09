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
