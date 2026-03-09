export type AnswerDraftSource = "text" | "voice_review";

export interface AnswerDraftSnapshot {
  turnId: string;
  questionText: string;
  draftText: string;
  source: AnswerDraftSource;
  updatedAt: number;
}

function answerDraftKey(sessionCode: string, turnId: string): string {
  return `interview_answer_draft_${sessionCode}_${turnId}`;
}

function answerDraftPrefix(sessionCode: string): string {
  return `interview_answer_draft_${sessionCode}_`;
}

function isAnswerDraftSnapshot(value: unknown): value is AnswerDraftSnapshot {
  if (!value || typeof value !== "object") return false;
  const draft = value as Partial<AnswerDraftSnapshot>;
  return (
    typeof draft.turnId === "string"
    && typeof draft.questionText === "string"
    && typeof draft.draftText === "string"
    && (draft.source === "text" || draft.source === "voice_review")
    && typeof draft.updatedAt === "number"
  );
}

export function read(sessionCode: string, turnId: string): AnswerDraftSnapshot | null {
  if (typeof window === "undefined") return null;

  const key = answerDraftKey(sessionCode, turnId);
  const raw = localStorage.getItem(key);
  if (!raw) return null;

  try {
    const parsed = JSON.parse(raw) as unknown;
    if (isAnswerDraftSnapshot(parsed) && parsed.turnId === turnId) {
      return parsed;
    }
  } catch {
    // Clear malformed payloads below.
  }

  localStorage.removeItem(key);
  return null;
}

export function write(sessionCode: string, draft: AnswerDraftSnapshot): void {
  if (typeof window === "undefined") return;
  localStorage.setItem(answerDraftKey(sessionCode, draft.turnId), JSON.stringify(draft));
}

export function clear(sessionCode: string, turnId: string): void {
  if (typeof window === "undefined") return;
  localStorage.removeItem(answerDraftKey(sessionCode, turnId));
}

export function clearStale(sessionCode: string, activeTurnId: string): void {
  if (typeof window === "undefined") return;

  const prefix = answerDraftPrefix(sessionCode);
  for (let index = localStorage.length - 1; index >= 0; index -= 1) {
    const key = localStorage.key(index);
    if (!key || !key.startsWith(prefix) || key === answerDraftKey(sessionCode, activeTurnId)) {
      continue;
    }
    localStorage.removeItem(key);
  }
}
