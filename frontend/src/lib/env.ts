// isDevEnv reports whether admin-only local tooling should be enabled.
// Next.js sets NODE_ENV to "development" when running `next dev`.
export function isDevEnv(): boolean {
  return process.env.NODE_ENV === "development";
}
