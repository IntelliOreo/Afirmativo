import { describe, expect, it } from "vitest";
import { getTimeoutDialogState } from "./getTimeoutDialogState";
import type { Question } from "../models";
import type { TurnId } from "../models";
import type { InputMode, VoiceRecorderState } from "../viewTypes";

function makeQuestion(overrides: Partial<Question> = {}): Question {
  return {
    textEs: "Pregunta",
    textEn: "Question",
    area: "protected_ground",
    kind: "criterion",
    turnId: "turn-1" as TurnId,
    questionNumber: 2,
    totalQuestions: 25,
    ...overrides,
  };
}

interface Overrides {
  showTimeoutDialog?: boolean;
  phase?: "active" | "submitting";
  currentQuestion?: Question;
  isTimerExpired?: boolean;
  inputMode?: InputMode;
  voiceRecorderState?: VoiceRecorderState;
  textAnswer?: string;
  liveExpiredTurnId?: string;
  timeoutReviewTurnId?: string;
}

function run(overrides: Overrides = {}) {
  return getTimeoutDialogState({
    showTimeoutDialog: true,
    phase: "active",
    currentQuestion: makeQuestion(),
    isTimerExpired: true,
    inputMode: "text",
    voiceRecorderState: "idle",
    textAnswer: "",
    liveExpiredTurnId: "turn-1",
    timeoutReviewTurnId: "",
    ...overrides,
  });
}

describe("getTimeoutDialogState", () => {
  describe("shouldShowTimeoutDialog", () => {
    it("returns false when showTimeoutDialog is false and not in voice review_ready", () => {
      const result = run({ showTimeoutDialog: false });
      expect(result.shouldShowTimeoutDialog).toBe(false);
    });

    it("returns true for criterion question with expired timer and dialog open", () => {
      const result = run();
      expect(result.shouldShowTimeoutDialog).toBe(true);
    });

    it("returns true for voice review_ready even without explicit dialog trigger", () => {
      const result = run({
        showTimeoutDialog: false,
        inputMode: "voice",
        voiceRecorderState: "review_ready",
      });
      expect(result.shouldShowTimeoutDialog).toBe(true);
    });

    it("returns false when phase is submitting", () => {
      const result = run({ phase: "submitting" });
      expect(result.shouldShowTimeoutDialog).toBe(false);
    });

    it("returns false for readiness questions", () => {
      const result = run({ currentQuestion: makeQuestion({ kind: "readiness" }) });
      expect(result.shouldShowTimeoutDialog).toBe(false);
    });

    it("returns false for disclaimer questions", () => {
      const result = run({ currentQuestion: makeQuestion({ kind: "disclaimer" }) });
      expect(result.shouldShowTimeoutDialog).toBe(false);
    });

    it("returns false when timer has not expired", () => {
      const result = run({ isTimerExpired: false });
      expect(result.shouldShowTimeoutDialog).toBe(false);
    });
  });

  describe("shouldAutoReviewTimedOutVoice", () => {
    it("returns true when voice is audio_ready, expired, and not yet reviewed", () => {
      const result = run({
        inputMode: "voice",
        voiceRecorderState: "audio_ready",
        timeoutReviewTurnId: "",
      });
      expect(result.shouldAutoReviewTimedOutVoice).toBe(true);
    });

    it("returns false when timeoutReviewTurnId matches current turn", () => {
      const result = run({
        inputMode: "voice",
        voiceRecorderState: "audio_ready",
        timeoutReviewTurnId: "turn-1",
      });
      expect(result.shouldAutoReviewTimedOutVoice).toBe(false);
    });

    it("returns false in text mode even with audio_ready state", () => {
      const result = run({
        inputMode: "text",
        voiceRecorderState: "audio_ready",
      });
      expect(result.shouldAutoReviewTimedOutVoice).toBe(false);
    });
  });

  describe("isTimeoutDialogTranscribing", () => {
    it("returns true during transcribing_for_review state", () => {
      const result = run({
        inputMode: "voice",
        voiceRecorderState: "transcribing_for_review",
      });
      expect(result.isTimeoutDialogTranscribing).toBe(true);
    });

    it("returns true when auto-review is about to fire (recording state)", () => {
      const result = run({
        inputMode: "voice",
        voiceRecorderState: "recording",
      });
      expect(result.isTimeoutDialogTranscribing).toBe(true);
    });

    it("returns true when auto-review is about to fire (paused state)", () => {
      const result = run({
        inputMode: "voice",
        voiceRecorderState: "paused",
      });
      expect(result.isTimeoutDialogTranscribing).toBe(true);
    });

    it("returns true when shouldAutoReviewTimedOutVoice is true (audio_ready)", () => {
      const result = run({
        inputMode: "voice",
        voiceRecorderState: "audio_ready",
        timeoutReviewTurnId: "",
      });
      expect(result.isTimeoutDialogTranscribing).toBe(true);
    });

    it("returns false in text mode", () => {
      const result = run({ inputMode: "text" });
      expect(result.isTimeoutDialogTranscribing).toBe(false);
    });
  });

  describe("isTimeoutDialogInterrupted", () => {
    it("returns true when no usable text/voice and liveExpiredTurnId does not match (page reload)", () => {
      const result = run({
        textAnswer: "",
        voiceRecorderState: "idle",
        liveExpiredTurnId: "",
      });
      expect(result.isTimeoutDialogInterrupted).toBe(true);
    });

    it("returns false when there is usable text", () => {
      const result = run({
        textAnswer: "Some answer",
        liveExpiredTurnId: "",
      });
      expect(result.isTimeoutDialogInterrupted).toBe(false);
    });

    it("returns false when liveExpiredTurnId matches current turn (expired live, not reloaded)", () => {
      const result = run({
        textAnswer: "",
        voiceRecorderState: "idle",
        liveExpiredTurnId: "turn-1",
      });
      expect(result.isTimeoutDialogInterrupted).toBe(false);
    });

    it("returns false when voice has recoverable state (audio_ready)", () => {
      const result = run({
        textAnswer: "",
        inputMode: "voice",
        voiceRecorderState: "audio_ready",
        liveExpiredTurnId: "",
        timeoutReviewTurnId: "turn-1",
      });
      expect(result.isTimeoutDialogInterrupted).toBe(false);
    });

    it("returns false when dialog is transcribing", () => {
      const result = run({
        textAnswer: "",
        inputMode: "voice",
        voiceRecorderState: "transcribing_for_review",
        liveExpiredTurnId: "",
      });
      expect(result.isTimeoutDialogInterrupted).toBe(false);
    });
  });
});
