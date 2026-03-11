export type InterviewPhase = "guard" | "loading" | "active" | "submitting" | "done" | "error";
export type SubmitMode = "question" | "finalAuto";
export type ReportStatus = "idle" | "loading" | "generating" | "ready" | "error";
export type CompletionSource = "finished" | "already_completed";
export type InputMode = "text" | "voice";
export type VoiceRecorderState =
  | "idle"
  | "recording"
  | "paused"
  | "audio_ready"
  | "transcribing_for_review"
  | "review_ready"
  | "forced_finalizing";

export type MicWarmState =
  | "cold"
  | "warming"
  | "warm"
  | "recovering"
  | "denied"
  | "error";

export type MicrophoneWarmupDialogMode = "initial_setup" | "reconnect";
export type MicrophoneWarmupDialogState =
  | "idle"
  | "warming"
  | "recovering"
  | "ready_handoff"
  | "denied"
  | "error";

export interface VoiceCapabilities {
  canSwitchModes: boolean;
  canToggleRecording: boolean;
  canCompleteRecording: boolean;
  canDiscardRecording: boolean;
  canReviewTranscript: boolean;
  canSubmitAnswer: boolean;
  canPreviewRecording: boolean;
  centerControlLabel: string;
}

export type CodedError = Error & { code?: string };

export type DisclaimerBlock =
  | { type: "paragraph"; text: string }
  | { type: "list"; items: string[] };
