export type InterviewPhase = "guard" | "loading" | "active" | "submitting" | "done" | "error";
export type SubmitMode = "question";
export type ReportStatus = "idle" | "loading" | "generating" | "ready" | "error";
export type CompletionSource = "finished" | "already_completed";
export type InputMode = "text" | "voice";
export type VoiceRecorderState =
  | "idle"
  | "recording"
  | "paused"
  | "audio_ready"
  | "transcribing_for_review"
  | "review_ready";

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

export type ReportErrorCode =
  | "load_failed"
  | "queue_failed"
  | "generation_failed"
  | "polling_timed_out"
  | "polling_paused"
  | "network"
  | "unknown";

export interface ReportErrorState {
  code: ReportErrorCode;
  requestId?: string;
}

export type VoiceFeedbackCode =
  | "microphone_permission_denied"
  | "microphone_unavailable"
  | "browser_unsupported"
  | "secure_context_required"
  | "switch_mode_while_recording"
  | "recording_failed"
  | "no_audio_detected"
  | "audio_playback_failed"
  | "transcription_failed"
  | "session_unauthorized"
  | "voice_api_unavailable"
  | "limit_reached"
  | "audio_ready"
  | "preparing_transcript"
  | "transcript_ready";

export interface VoiceFeedback {
  code: VoiceFeedbackCode;
  requestId?: string;
}
