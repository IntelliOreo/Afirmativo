import { describe, expect, it } from "vitest";
import {
  getVoiceCapabilities,
  isCompletedResponse,
  isUnauthorizedResponse,
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
    expect(caps.canCompleteRecording).toBe(false);
    expect(caps.canDiscardRecording).toBe(false);
    expect(caps.canReviewTranscript).toBe(false);
    expect(caps.canSubmitAnswer).toBe(false);
    expect(caps.canPreviewRecording).toBe(false);
    expect(caps.centerControlLabel).toBe("Record");
  });

  it("blocks mode switching while recording and allows record controls", () => {
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
    expect(caps.canCompleteRecording).toBe(true);
    expect(caps.canDiscardRecording).toBe(true);
    expect(caps.centerControlLabel).toBe("Pause");
  });

  it("only enables preview and send once a stopped recording is ready", () => {
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
    expect(caps.canCompleteRecording).toBe(false);
    expect(caps.canDiscardRecording).toBe(true);
    expect(caps.canReviewTranscript).toBe(true);
    expect(caps.canSubmitAnswer).toBe(false);
    expect(caps.canPreviewRecording).toBe(true);
    expect(caps.centerControlLabel).toBe("Record");
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
    expect(caps.canCompleteRecording).toBe(false);
    expect(caps.canDiscardRecording).toBe(false);
    expect(caps.canReviewTranscript).toBe(false);
    expect(caps.canSubmitAnswer).toBe(false);
    expect(caps.canPreviewRecording).toBe(false);
  });
});
