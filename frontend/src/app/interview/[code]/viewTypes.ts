export type InterviewPhase = "guard" | "loading" | "active" | "submitting" | "done" | "error";
export type SubmitMode = "question" | "finalAuto";
export type ReportStatus = "idle" | "loading" | "generating" | "ready" | "error";
export type CompletionSource = "finished" | "already_completed";
export type InputMode = "text" | "voice";
export type VoiceRecorderState = "idle" | "recording" | "paused" | "stopped" | "sending";

export interface VoiceCapabilities {
  canSwitchModes: boolean;
  canToggleRecording: boolean;
  canCompleteRecording: boolean;
  canDiscardRecording: boolean;
  canSendRecording: boolean;
  canPreviewRecording: boolean;
  centerControlLabel: string;
}

export type CodedError = Error & { code?: string };

export type DisclaimerBlock =
  | { type: "paragraph"; text: string }
  | { type: "list"; items: string[] };
