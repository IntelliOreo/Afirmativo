import type {
  CodedError,
  DisclaimerBlock,
  PendingAnswerJob,
  Question,
} from "./types";

export function getQuestionTextForLang(
  q: Question | null | undefined,
  language: "es" | "en",
): string {
  if (!q) return "";
  if (language === "es") return q.textEs || q.textEn || "";
  return q.textEn || q.textEs || "";
}

export function parseDisclaimerBlocks(rawText: string): DisclaimerBlock[] {
  const lines = rawText.split("\n");
  const blocks: DisclaimerBlock[] = [];
  let currentListItems: string[] = [];

  const flushCurrentList = () => {
    if (currentListItems.length > 0) {
      blocks.push({ type: "list", items: currentListItems });
      currentListItems = [];
    }
  };

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) {
      flushCurrentList();
      continue;
    }

    if (trimmed.startsWith("- ") || trimmed.startsWith("• ")) {
      currentListItems.push(trimmed.replace(/^[-•]\s+/, ""));
      continue;
    }

    flushCurrentList();
    blocks.push({ type: "paragraph", text: trimmed });
  }

  flushCurrentList();
  return blocks;
}

export function wait(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export function formatClock(totalSeconds: number): string {
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const kb = bytes / 1024;
  if (kb < 1024) return `${kb.toFixed(1)} KB`;
  return `${(kb / 1024).toFixed(1)} MB`;
}

export function withJitter(ms: number): number {
  const jitterFactor = 0.85 + Math.random() * 0.3;
  return Math.max(250, Math.round(ms * jitterFactor));
}

export function makeClientRequestId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export function pendingJobStorageKey(sessionCode: string): string {
  return `interview_pending_answer_job_${sessionCode}`;
}

export function readPendingAnswerJob(sessionCode: string): PendingAnswerJob | null {
  if (typeof window === "undefined") return null;
  const raw = localStorage.getItem(pendingJobStorageKey(sessionCode));
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw) as PendingAnswerJob;
    if (!parsed?.clientRequestId || !parsed?.turnId) return null;
    return parsed;
  } catch {
    return null;
  }
}

export function writePendingAnswerJob(sessionCode: string, pending: PendingAnswerJob): void {
  if (typeof window === "undefined") return;
  localStorage.setItem(pendingJobStorageKey(sessionCode), JSON.stringify(pending));
}

export function clearPendingAnswerJob(sessionCode: string): void {
  if (typeof window === "undefined") return;
  localStorage.removeItem(pendingJobStorageKey(sessionCode));
}

export function buildCodedError(message: string, code?: string): CodedError {
  const err = new Error(message) as CodedError;
  if (code) {
    err.code = code;
  }
  return err;
}

export function extractErrorCode(err: unknown): string {
  if (!err || typeof err !== "object" || !("code" in err)) {
    return "";
  }
  const maybeCode = (err as { code?: unknown }).code;
  return typeof maybeCode === "string" ? maybeCode : "";
}

export function isReloadRecoveryErrorCode(errorCode: string): boolean {
  return errorCode === "TURN_CONFLICT" || errorCode === "IDEMPOTENCY_CONFLICT";
}

export function shouldClearPendingAnswerOnError(errorCode: string): boolean {
  return errorCode === "IDEMPOTENCY_CONFLICT";
}

export function randomMessageIndex(currentIndex: number, total: number): number {
  if (total <= 1) return 0;
  let nextIndex = Math.floor(Math.random() * total);
  while (nextIndex === currentIndex) {
    nextIndex = Math.floor(Math.random() * total);
  }
  return nextIndex;
}
