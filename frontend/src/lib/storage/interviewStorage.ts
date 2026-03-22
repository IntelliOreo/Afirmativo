const INTERVIEW_STORAGE_PREFIXES = [
  "interview_answer_draft_",
  "interview_pending_answer_job_",
] as const;

export function clearAllInterviewStorage(): void {
  if (typeof window === "undefined") return;

  const keysToRemove: string[] = [];
  for (let index = 0; index < localStorage.length; index += 1) {
    const key = localStorage.key(index);
    if (!key) continue;
    if (INTERVIEW_STORAGE_PREFIXES.some((prefix) => key.startsWith(prefix))) {
      keysToRemove.push(key);
    }
  }

  keysToRemove.forEach((key) => {
    localStorage.removeItem(key);
  });
}

export function isQuotaExceededError(error: unknown): boolean {
  if (!error || typeof error !== "object") return false;

  const maybeName = "name" in error ? String(error.name) : "";
  const maybeCode = "code" in error ? Number(error.code) : 0;

  return maybeName === "QuotaExceededError"
    || maybeName === "NS_ERROR_DOM_QUOTA_REACHED"
    || maybeCode === 22
    || maybeCode === 1014;
}
