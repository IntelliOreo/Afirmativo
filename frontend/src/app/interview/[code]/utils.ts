import type {
  CodedError,
  DisclaimerBlock,
  VoiceCapabilities,
  VoiceRecorderState,
  InterviewPhase,
} from "./viewTypes";

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

const EC_UNAUTHORIZED = "UNAUTHORIZED";
const EC_SESSION_MISMATCH = "SESSION_MISMATCH";
const EC_INTERVIEW_COMPLETED = "INTERVIEW_COMPLETED";
const EC_TURN_CONFLICT = "TURN_CONFLICT";
const EC_IDEMPOTENCY_CONFLICT = "IDEMPOTENCY_CONFLICT";
const EC_ASYNC_POLL_TIMEOUT = "ASYNC_POLL_TIMEOUT";
const EC_ASYNC_POLL_CIRCUIT_OPEN = "ASYNC_POLL_CIRCUIT_OPEN";
const EC_AI_RETRY_EXHAUSTED = "AI_RETRY_EXHAUSTED";
const EC_SESSION_EXPIRED = "SESSION_EXPIRED";

export function isUnauthorizedResponse(httpStatus: number, code?: string): boolean {
  return httpStatus === 401 || code === EC_UNAUTHORIZED || code === EC_SESSION_MISMATCH;
}

export function isCompletedResponse(httpStatus: number, code?: string, message?: string): boolean {
  return httpStatus === 409
    || code === EC_INTERVIEW_COMPLETED
    || (message ?? "").toLowerCase().includes("completed");
}

export function isReloadRecoveryErrorCode(errorCode: string): boolean {
  return errorCode === EC_TURN_CONFLICT
    || errorCode === EC_IDEMPOTENCY_CONFLICT
    || errorCode === EC_ASYNC_POLL_TIMEOUT
    || errorCode === EC_ASYNC_POLL_CIRCUIT_OPEN
    || errorCode === EC_AI_RETRY_EXHAUSTED;
}

export function isPendingRecoveryRetryableErrorCode(errorCode: string): boolean {
  return errorCode === ""
    || errorCode === EC_ASYNC_POLL_TIMEOUT
    || errorCode === EC_ASYNC_POLL_CIRCUIT_OPEN;
}

export function shouldAttemptStartAfterRecoveryError(errorCode: string): boolean {
  return errorCode === EC_TURN_CONFLICT
    || errorCode === EC_IDEMPOTENCY_CONFLICT
    || errorCode === EC_AI_RETRY_EXHAUSTED;
}

export function shouldClearPendingAnswerOnError(errorCode: string): boolean {
  return errorCode === EC_IDEMPOTENCY_CONFLICT
    || errorCode === EC_TURN_CONFLICT
    || errorCode === EC_SESSION_EXPIRED
    || errorCode === EC_UNAUTHORIZED
    || errorCode === EC_SESSION_MISMATCH;
}

export function getVoiceCapabilities(params: {
  phase: InterviewPhase;
  voiceRecorderState: VoiceRecorderState;
  voiceBlob: Blob | null;
  voicePreviewUrl: string | null;
  hasDraftText: boolean;
  isFinalReviewWindow: boolean;
}): VoiceCapabilities {
  const {
    phase,
    voiceRecorderState,
    voiceBlob,
    voicePreviewUrl,
    hasDraftText,
    isFinalReviewWindow,
  } = params;

  const isRecordingState = voiceRecorderState === "recording" || voiceRecorderState === "paused";
  const isBusyState = voiceRecorderState === "transcribing_for_review";

  return {
    canSwitchModes:
      phase === "active"
      && !isFinalReviewWindow
      && !isRecordingState
      && !isBusyState,
    canToggleRecording:
      phase === "active"
      && (
        voiceRecorderState === "recording"
        || voiceRecorderState === "paused"
        || voiceRecorderState === "idle"
      ),
    canCompleteRecording:
      phase === "active"
      && isRecordingState,
    canDiscardRecording:
      phase === "active"
      && (
        voiceRecorderState === "recording"
        || voiceRecorderState === "paused"
        || voiceRecorderState === "audio_ready"
        || voiceRecorderState === "review_ready"
      ),
    canReviewTranscript:
      phase === "active"
      && voiceRecorderState === "audio_ready"
      && !!voiceBlob
      && voiceBlob.size > 0,
    canSubmitAnswer:
      phase === "active"
      && voiceRecorderState === "review_ready"
      && hasDraftText,
    canPreviewRecording:
      phase === "active"
      && (
        voiceRecorderState === "paused"
        || voiceRecorderState === "audio_ready"
        || voiceRecorderState === "review_ready"
      )
      && !!voicePreviewUrl,
    centerControlLabel:
      voiceRecorderState === "idle"
        ? "Record"
        : voiceRecorderState === "recording"
          ? "Pause"
          : voiceRecorderState === "paused"
            ? "Resume"
            : "Record",
  };
}

export function randomMessageIndex(currentIndex: number, total: number): number {
  if (total <= 1) return 0;
  let nextIndex = Math.floor(Math.random() * total);
  while (nextIndex === currentIndex) {
    nextIndex = Math.floor(Math.random() * total);
  }
  return nextIndex;
}
