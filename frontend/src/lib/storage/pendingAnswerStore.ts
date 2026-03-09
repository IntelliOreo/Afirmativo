import type { PendingAnswerSubmission } from "@/app/interview/[code]/models";

function pendingAnswerKey(sessionCode: string): string {
  return `interview_pending_answer_job_${sessionCode}`;
}

function isPendingAnswerSubmission(value: unknown): value is PendingAnswerSubmission {
  if (!value || typeof value !== "object") return false;
  const job = value as Partial<PendingAnswerSubmission>;
  return (
    typeof job.clientRequestId === "string"
    && typeof job.turnId === "string"
    && typeof job.answerText === "string"
    && typeof job.questionText === "string"
    && typeof job.createdAt === "number"
    && (job.jobId === undefined || typeof job.jobId === "string")
  );
}

export function read(sessionCode: string): PendingAnswerSubmission | null {
  if (typeof window === "undefined") return null;

  const key = pendingAnswerKey(sessionCode);
  const raw = localStorage.getItem(key);
  if (!raw) return null;

  try {
    const parsed = JSON.parse(raw) as unknown;
    if (isPendingAnswerSubmission(parsed)) {
      return parsed;
    }
  } catch {
    // Clear malformed payloads below.
  }

  localStorage.removeItem(key);
  return null;
}

export function write(sessionCode: string, pendingJob: PendingAnswerSubmission): void {
  if (typeof window === "undefined") return;
  localStorage.setItem(pendingAnswerKey(sessionCode), JSON.stringify(pendingJob));
}

export function clear(sessionCode: string): void {
  if (typeof window === "undefined") return;
  localStorage.removeItem(pendingAnswerKey(sessionCode));
}
