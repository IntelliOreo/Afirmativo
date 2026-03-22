import { describe, expect, it } from "vitest";
import {
  getVoiceCapabilities,
  isCompletedResponse,
  isPendingRecoveryRetryableErrorCode,
  isReloadRecoveryErrorCode,
  isUnauthorizedResponse,
  shouldAttemptStartAfterRecoveryError,
  shouldClearPendingAnswerOnError,
} from "./utils";

describe("isUnauthorizedResponse", () => {
  it("treats http 401 and auth codes as unauthorized", () => {
    expect(isUnauthorizedResponse(401)).toBe(true);
    expect(isUnauthorizedResponse(200, "UNAUTHORIZED")).toBe(true);
    expect(isUnauthorizedResponse(200, "SESSION_MISMATCH")).toBe(true);
  });

  it("does not mark unrelated responses as unauthorized", () => {
    expect(isUnauthorizedResponse(500)).toBe(false);
    expect(isUnauthorizedResponse(409, "INTERVIEW_COMPLETED")).toBe(false);
  });
});

describe("isCompletedResponse", () => {
  it("treats completion statuses, codes, and messages as completed", () => {
    expect(isCompletedResponse(409)).toBe(true);
    expect(isCompletedResponse(200, "INTERVIEW_COMPLETED")).toBe(true);
    expect(isCompletedResponse(200, undefined, "Already COMPLETED")).toBe(true);
  });

  it("does not mark unrelated errors as completed", () => {
    expect(isCompletedResponse(500, "UNAUTHORIZED", "failed to start")).toBe(false);
  });
});

describe("getVoiceCapabilities", () => {
  it("enables idle recording controls while interview is active", () => {
    const caps = getVoiceCapabilities({
      phase: "active",
      voiceRecorderState: "idle",
      voiceBlob: null,
      voicePreviewUrl: null,
      hasDraftText: false,
      isFinalReviewWindow: false,
    });

    expect(caps.canSwitchModes).toBe(true);
    expect(caps.canToggleRecording).toBe(true);
    expect(caps.canReplayRecording).toBe(false);
    expect(caps.canDiscardRecording).toBe(false);
    expect(caps.canReviewTranscript).toBe(false);
    expect(caps.canSubmitAnswer).toBe(false);
    expect(caps.centerControlLabel).toBe("Record");
  });

  it("blocks mode switching while recording and enables direct review", () => {
    const caps = getVoiceCapabilities({
      phase: "active",
      voiceRecorderState: "recording",
      voiceBlob: null,
      voicePreviewUrl: null,
      hasDraftText: false,
      isFinalReviewWindow: false,
    });

    expect(caps.canSwitchModes).toBe(false);
    expect(caps.canToggleRecording).toBe(true);
    expect(caps.canReplayRecording).toBe(false);
    expect(caps.canDiscardRecording).toBe(true);
    expect(caps.canReviewTranscript).toBe(true);
    expect(caps.centerControlLabel).toBe("Pause");
  });

  it("enables replay and transcript review once a stopped recording is ready", () => {
    const caps = getVoiceCapabilities({
      phase: "active",
      voiceRecorderState: "audio_ready",
      voiceBlob: new Blob(["audio"]),
      voicePreviewUrl: "blob:preview",
      hasDraftText: false,
      isFinalReviewWindow: false,
    });

    expect(caps.canSwitchModes).toBe(true);
    expect(caps.canToggleRecording).toBe(false);
    expect(caps.canReplayRecording).toBe(true);
    expect(caps.canDiscardRecording).toBe(true);
    expect(caps.canReviewTranscript).toBe(true);
    expect(caps.canSubmitAnswer).toBe(false);
    expect(caps.centerControlLabel).toBe("Record");
  });

  it("allows discard and re-record during final review window", () => {
    const caps = getVoiceCapabilities({
      phase: "active",
      voiceRecorderState: "recording",
      voiceBlob: null,
      voicePreviewUrl: null,
      hasDraftText: false,
      isFinalReviewWindow: true,
    });

    expect(caps.canSwitchModes).toBe(false);
    expect(caps.canToggleRecording).toBe(true);
    expect(caps.canReplayRecording).toBe(false);
    expect(caps.canDiscardRecording).toBe(true);
    expect(caps.canReviewTranscript).toBe(true);
  });

  it("allows starting a new recording from idle during final review window", () => {
    const caps = getVoiceCapabilities({
      phase: "active",
      voiceRecorderState: "idle",
      voiceBlob: null,
      voicePreviewUrl: null,
      hasDraftText: false,
      isFinalReviewWindow: true,
    });

    expect(caps.canSwitchModes).toBe(false);
    expect(caps.canToggleRecording).toBe(true);
    expect(caps.canDiscardRecording).toBe(false);
  });

  it("blocks sending empty audio and all controls when interview is inactive", () => {
    const caps = getVoiceCapabilities({
      phase: "done",
      voiceRecorderState: "audio_ready",
      voiceBlob: new Blob([]),
      voicePreviewUrl: "blob:preview",
      hasDraftText: false,
      isFinalReviewWindow: false,
    });

    expect(caps.canSwitchModes).toBe(false);
    expect(caps.canToggleRecording).toBe(false);
    expect(caps.canReplayRecording).toBe(false);
    expect(caps.canDiscardRecording).toBe(false);
    expect(caps.canReviewTranscript).toBe(false);
    expect(caps.canSubmitAnswer).toBe(false);
  });
});

describe("isReloadRecoveryErrorCode", () => {
  it("treats reload-only recovery failures as reloadable", () => {
    expect(isReloadRecoveryErrorCode("AI_RETRY_EXHAUSTED")).toBe(true);
  });

  it("does not mark unrelated codes as reload-only", () => {
    expect(isReloadRecoveryErrorCode("INTERNAL_ERROR")).toBe(false);
  });
});

describe("isPendingRecoveryRetryableErrorCode", () => {
  it("treats transient pending recovery errors as retryable", () => {
    expect(isPendingRecoveryRetryableErrorCode("")).toBe(true);
    expect(isPendingRecoveryRetryableErrorCode("ASYNC_POLL_TIMEOUT")).toBe(true);
  });

  it("does not mark terminal flow conflicts as retryable", () => {
    expect(isPendingRecoveryRetryableErrorCode("TURN_CONFLICT")).toBe(false);
  });
});

describe("shouldAttemptStartAfterRecoveryError", () => {
  it("falls back to start for terminal recovery mismatches", () => {
    expect(shouldAttemptStartAfterRecoveryError("TURN_CONFLICT")).toBe(true);
    expect(shouldAttemptStartAfterRecoveryError("AI_RETRY_EXHAUSTED")).toBe(true);
  });

  it("does not fall back to start for transient recovery errors", () => {
    expect(shouldAttemptStartAfterRecoveryError("ASYNC_POLL_TIMEOUT")).toBe(false);
  });
});

describe("shouldClearPendingAnswerOnError", () => {
  it("clears persisted pending state for confirmed stale or auth errors", () => {
    expect(shouldClearPendingAnswerOnError("TURN_CONFLICT")).toBe(true);
    expect(shouldClearPendingAnswerOnError("SESSION_EXPIRED")).toBe(true);
  });

  it("keeps persisted pending state for retryable errors", () => {
    expect(shouldClearPendingAnswerOnError("AI_RETRY_EXHAUSTED")).toBe(false);
    expect(shouldClearPendingAnswerOnError("ASYNC_POLL_TIMEOUT")).toBe(false);
  });
});
