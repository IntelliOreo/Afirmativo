"use client";

import { Alert } from "@components/Alert";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import type { Lang } from "@/lib/language";
import { formatDurationLabel, VOICE_MAX_SECONDS, VOICE_WAVE_BARS } from "../constants";
import {
  getInterviewMessages,
  getMicSetupCopy,
  getRecorderStatusMessage,
} from "../messages/interviewMessages";
import type { MicWarmState, VoiceRecorderState } from "../viewTypes";
import { formatBytes } from "../utils";

interface VoiceRecorderPanelProps {
  lang: Lang;
  hasMicOptIn: boolean;
  micWarmState: MicWarmState;
  onPrepareMicrophone: () => Promise<void>;
  answerTimerLabel: string;
  answerTimerTone: "normal" | "warning" | "danger";
  answerTimerMessage: string;
  voiceTimerLabel: string;
  canPreviewRecording: boolean;
  isVoicePreviewPlaying: boolean;
  onToggleVoicePreviewPlayback: () => Promise<void>;
  voiceIsRecordingActive: boolean;
  voiceProgressPct: number;
  voiceWarningRemaining: number | null;
  voiceReviewWarning: string;
  voiceError: string;
  voiceInfo: string;
  voiceRecorderState: VoiceRecorderState;
  voiceIsRecordingPaused: boolean;
  voiceBlob: Blob | null;
  canDiscardRecording: boolean;
  onDiscardVoiceRecording: () => void;
  canToggleRecording: boolean;
  onStartVoiceRecording: () => Promise<void>;
  centerControlLabel: string;
  canCompleteRecording: boolean;
  onCompleteVoiceRecording: () => void;
  canReviewTranscript: boolean;
  onReviewVoiceAnswer: () => Promise<void>;
  canSubmitAnswer: boolean;
  isTimerExpired?: boolean;
  transcriptText: string;
  onTranscriptChange: (nextValue: string) => void;
  onSubmitAnswer: () => Promise<void> | void;
}

export function VoiceRecorderPanel({
  lang,
  hasMicOptIn,
  micWarmState,
  onPrepareMicrophone,
  answerTimerLabel,
  answerTimerTone,
  answerTimerMessage,
  voiceTimerLabel,
  canPreviewRecording,
  isVoicePreviewPlaying,
  onToggleVoicePreviewPlayback,
  voiceIsRecordingActive,
  voiceProgressPct,
  voiceWarningRemaining,
  voiceReviewWarning,
  voiceError,
  voiceInfo,
  voiceRecorderState,
  voiceIsRecordingPaused,
  voiceBlob,
  canDiscardRecording,
  onDiscardVoiceRecording,
  canToggleRecording,
  onStartVoiceRecording,
  centerControlLabel,
  canCompleteRecording,
  onCompleteVoiceRecording,
  canReviewTranscript,
  onReviewVoiceAnswer,
  canSubmitAnswer,
  isTimerExpired = false,
  transcriptText,
  onTranscriptChange,
  onSubmitAnswer,
}: VoiceRecorderPanelProps) {
  const t = getInterviewMessages(lang).voice;
  const voiceLimitLabel = formatDurationLabel(VOICE_MAX_SECONDS, lang);
  const micSetupCopy = getMicSetupCopy(lang, hasMicOptIn, micWarmState);
  const shouldShowMicSetup =
    voiceRecorderState === "idle"
    && !voiceBlob
    && (micWarmState !== "warm" || !hasMicOptIn);
  const timerToneClass =
    answerTimerTone === "danger"
      ? "border-danger bg-danger-lightest text-danger-dark"
      : answerTimerTone === "warning"
        ? "border-yellow-300 bg-yellow-50 text-yellow-900"
        : "border-primary/20 bg-primary/5 text-primary-darkest";

  const isTranscriptVisible =
    voiceRecorderState === "review_ready";
  const primaryButtonLabel =
    voiceRecorderState === "transcribing_for_review"
      ? t.reviewingTranscript
      : voiceRecorderState === "review_ready"
        ? t.submitAnswer
        : t.reviewTranscript;

  return (
    <Card className="mb-4">
      <div className={`mb-4 rounded-lg border px-4 py-3 ${timerToneClass}`}>
        <p className="text-xs font-semibold uppercase tracking-wide">
          {t.submitWindowLabel}
        </p>
        <p className="mt-1 text-2xl font-bold">{answerTimerLabel}</p>
        <p className="mt-2 text-sm leading-snug">{answerTimerMessage}</p>
      </div>

      <div className="mb-4 flex items-center justify-center gap-3">
        <p className="text-center text-5xl font-bold tracking-wide text-primary">
          {voiceTimerLabel}
        </p>
        <button
          type="button"
          className="h-9 w-9 rounded-full border border-primary text-primary text-sm font-bold disabled:opacity-40 disabled:cursor-not-allowed"
          aria-label={t.playbackAria}
          disabled={!canPreviewRecording}
          onClick={() => { void onToggleVoicePreviewPlayback(); }}
        >
          {isVoicePreviewPlaying ? "II" : ">"}
        </button>
      </div>

      <div className="mb-5 flex items-end justify-center gap-1 h-8">
        {VOICE_WAVE_BARS.map((bar, index) => (
          <span
            key={`voice-wave-${index}`}
            className={`w-1.5 rounded-full transition-colors ${
              voiceIsRecordingActive ? "bg-primary-dark" : "bg-primary/50"
            }`}
            style={{ height: `${bar}px` }}
          />
        ))}
      </div>

      <div className="h-2 bg-base-lighter rounded mb-5">
        <div
          className="h-2 bg-primary rounded transition-all duration-200"
          style={{ width: `${voiceProgressPct}%` }}
        />
      </div>

      {shouldShowMicSetup && (
        <Alert variant={micSetupCopy.variant} className="mb-4">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <span>{micSetupCopy.message}</span>
            <Button
              type="button"
              variant="secondary"
              className="sm:shrink-0"
              onClick={() => { void onPrepareMicrophone(); }}
              disabled={micSetupCopy.busy}
            >
              {micSetupCopy.buttonLabel}
            </Button>
          </div>
        </Alert>
      )}

      {voiceReviewWarning && (
        <Alert variant="warning" className="mb-4">
          {voiceReviewWarning}
        </Alert>
      )}

      {voiceWarningRemaining !== null && (
        <Alert variant="warning" className="mb-4">
          {t.warningBeforeLimit(voiceWarningRemaining, voiceLimitLabel)}
        </Alert>
      )}

      {voiceError && (
        <Alert variant="error" className="mb-4">
          {voiceError}
        </Alert>
      )}

      {voiceInfo && (
        <Alert variant="warning" className="mb-4">
          {voiceInfo}
        </Alert>
      )}

      <p className="text-center text-sm text-primary-darkest mb-4">
        {getRecorderStatusMessage(
          lang,
          voiceRecorderState,
          voiceIsRecordingPaused,
          voiceIsRecordingActive,
        )}
      </p>

      {voiceBlob && (
        <p className="text-sm text-primary-darkest mb-4">
          {t.audioReadyLabel}: {formatBytes(voiceBlob.size)}
        </p>
      )}

      {isTranscriptVisible && (
        <div className="mb-5">
          <label className="block font-semibold text-primary-darkest mb-2">
            {t.reviewableTranscript}
          </label>
          <textarea
            value={transcriptText}
            onChange={(event) => onTranscriptChange(event.target.value)}
            rows={6}
            className="w-full px-3 py-3 text-base border border-base-lighter rounded focus:outline-none focus:ring-2 focus:ring-primary resize-none"
            placeholder={t.transcriptPlaceholder}
            readOnly={isTimerExpired}
          />
        </div>
      )}

      <div className="mb-5 flex items-start justify-center gap-6 sm:gap-10">
        <div className="flex flex-col items-center gap-2">
          <Button
            type="button"
            variant="secondary"
            className="!h-14 !w-14 !rounded-full !px-0 !py-0 shadow-sm"
            disabled={!canDiscardRecording || isTimerExpired}
            onClick={onDiscardVoiceRecording}
          >
            ×
          </Button>
          <span className="text-xs font-semibold text-primary-darkest">
            {voiceRecorderState === "review_ready" || voiceRecorderState === "audio_ready"
              ? t.rerecord
              : t.discard}
          </span>
        </div>

        <div className="flex flex-col items-center gap-2">
          <Button
            type="button"
            variant="danger"
            className="!h-16 !w-16 !rounded-full !px-0 !py-0 shadow-md"
            disabled={!canToggleRecording || isTimerExpired}
            onClick={() => { void onStartVoiceRecording(); }}
          >
            {voiceIsRecordingActive ? "II" : voiceIsRecordingPaused ? ">" : "●"}
          </Button>
          <span className="text-xs font-semibold text-primary-darkest">
            {centerControlLabel}
          </span>
        </div>

        <div className="flex flex-col items-center gap-2">
          <Button
            type="button"
            variant="secondary"
            className="!h-14 !w-14 !rounded-full !px-0 !py-0 shadow-sm"
            disabled={!canCompleteRecording || isTimerExpired}
            onClick={onCompleteVoiceRecording}
          >
            ✓
          </Button>
          <span className="text-xs font-semibold text-primary-darkest">
            {t.complete}
          </span>
        </div>
      </div>

      <Button
        type="button"
        fullWidth
        disabled={
          voiceRecorderState === "review_ready"
            ? !canSubmitAnswer
            : !canReviewTranscript
        }
        onClick={() => {
          if (voiceRecorderState === "review_ready") {
            void onSubmitAnswer();
            return;
          }
          void onReviewVoiceAnswer();
        }}
      >
        {primaryButtonLabel}
      </Button>
    </Card>
  );
}
