import type { Question } from "../models";
import type { InputMode, VoiceRecorderState } from "../viewTypes";

interface GetTimeoutDialogStateParams {
  showTimeoutDialog: boolean;
  phase: "active" | "submitting";
  currentQuestion: Question;
  isTimerExpired: boolean;
  inputMode: InputMode;
  voiceRecorderState: VoiceRecorderState;
  textAnswer: string;
  liveExpiredTurnId: string;
  timeoutReviewTurnId: string;
}

interface TimeoutDialogState {
  shouldShowTimeoutDialog: boolean;
  shouldAutoReviewTimedOutVoice: boolean;
  isTimeoutDialogTranscribing: boolean;
  isTimeoutDialogInterrupted: boolean;
}

export function getTimeoutDialogState({
  showTimeoutDialog,
  phase,
  currentQuestion,
  isTimerExpired,
  inputMode,
  voiceRecorderState,
  textAnswer,
  liveExpiredTurnId,
  timeoutReviewTurnId,
}: GetTimeoutDialogStateParams): TimeoutDialogState {
  const isVoiceMode = inputMode === "voice";
  const hasRecoverableVoiceState =
    voiceRecorderState === "audio_ready"
    || voiceRecorderState === "transcribing_for_review"
    || voiceRecorderState === "review_ready";
  const hasUsableLocalRecoveryPath =
    textAnswer.trim().length > 0
    || hasRecoverableVoiceState;
  const shouldShowTimeoutDialog =
    showTimeoutDialog
    && phase === "active"
    && currentQuestion.kind === "criterion"
    && isTimerExpired;
  const shouldAutoReviewTimedOutVoice =
    shouldShowTimeoutDialog
    && isVoiceMode
    && voiceRecorderState === "audio_ready"
    && timeoutReviewTurnId !== currentQuestion.turnId;
  const isTimeoutDialogTranscribing =
    shouldShowTimeoutDialog
    && isVoiceMode
    && (
      voiceRecorderState === "recording"
      || voiceRecorderState === "paused"
      || voiceRecorderState === "transcribing_for_review"
      || shouldAutoReviewTimedOutVoice
    );
  const isTimeoutDialogInterrupted =
    shouldShowTimeoutDialog
    && !isTimeoutDialogTranscribing
    && !hasUsableLocalRecoveryPath
    && liveExpiredTurnId !== currentQuestion.turnId;

  return {
    shouldShowTimeoutDialog,
    shouldAutoReviewTimedOutVoice,
    isTimeoutDialogTranscribing,
    isTimeoutDialogInterrupted,
  };
}
