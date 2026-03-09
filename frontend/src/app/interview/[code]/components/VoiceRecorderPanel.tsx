"use client";

import { Alert } from "@components/Alert";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import type { Lang } from "@/lib/language";
import { VOICE_WAVE_BARS } from "../constants";
import type { VoiceRecorderState } from "../viewTypes";
import { formatBytes } from "../utils";

interface VoiceRecorderPanelProps {
  lang: Lang;
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
  transcriptText: string;
  onTranscriptChange: (nextValue: string) => void;
  onSubmitAnswer: () => Promise<void> | void;
}

function getRecorderStatusMessage(
  lang: Lang,
  voiceRecorderState: VoiceRecorderState,
  voiceIsRecordingPaused: boolean,
  voiceIsRecordingActive: boolean,
): string {
  if (voiceRecorderState === "idle") {
    return lang === "es" ? "Pulse Record para empezar." : "Press Record to begin.";
  }
  if (voiceIsRecordingPaused) {
    return lang === "es" ? "Grabación en pausa." : "Recording paused.";
  }
  if (voiceIsRecordingActive) {
    return lang === "es" ? "Grabando..." : "Recording...";
  }
  if (voiceRecorderState === "transcribing_for_review") {
    return lang === "es" ? "Preparando la transcripción..." : "Preparing transcript...";
  }
  if (voiceRecorderState === "review_ready") {
    return lang === "es"
      ? "Transcripción lista. Revísela y envíe su respuesta."
      : "Transcript ready. Review it and submit your answer.";
  }
  if (voiceRecorderState === "forced_finalizing") {
    return lang === "es" ? "Finalizando su respuesta..." : "Finalizing your answer...";
  }
  return lang === "es"
    ? "Audio listo. Revise la transcripción antes de enviar."
    : "Audio ready. Review the transcript before submitting.";
}

export function VoiceRecorderPanel({
  lang,
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
  transcriptText,
  onTranscriptChange,
  onSubmitAnswer,
}: VoiceRecorderPanelProps) {
  const timerToneClass =
    answerTimerTone === "danger"
      ? "border-danger bg-danger-lightest text-danger-dark"
      : answerTimerTone === "warning"
        ? "border-yellow-300 bg-yellow-50 text-yellow-900"
        : "border-primary/20 bg-primary/5 text-primary-darkest";

  const isTranscriptVisible =
    voiceRecorderState === "review_ready" || voiceRecorderState === "forced_finalizing";
  const primaryButtonLabel =
    voiceRecorderState === "transcribing_for_review"
      ? (lang === "es" ? "Revisando transcripción..." : "Reviewing transcript...")
      : voiceRecorderState === "review_ready"
        ? (lang === "es" ? "Enviar respuesta" : "Submit answer")
        : (lang === "es" ? "Revisar transcripción" : "Review transcript");

  return (
    <Card className="mb-4">
      <div className={`mb-4 rounded-lg border px-4 py-3 ${timerToneClass}`}>
        <p className="text-xs font-semibold uppercase tracking-wide">
          {lang === "es" ? "Envíe esta respuesta en" : "Submit this answer in"}
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
          aria-label={lang === "es" ? "Reproducir audio grabado" : "Play recorded audio"}
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

      {voiceReviewWarning && (
        <Alert variant="warning" className="mb-4">
          {voiceReviewWarning}
        </Alert>
      )}

      {voiceWarningRemaining !== null && (
        <Alert variant="warning" className="mb-4">
          {lang === "es"
            ? `Quedan ${voiceWarningRemaining}s para llegar al límite de 3 minutos.`
            : `${voiceWarningRemaining}s remain before the 3-minute limit.`}
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
          {lang === "es" ? "Audio listo" : "Audio ready"}: {formatBytes(voiceBlob.size)}
        </p>
      )}

      {isTranscriptVisible && (
        <div className="mb-5">
          <label className="block font-semibold text-primary-darkest mb-2">
            {lang === "es" ? "Transcripción revisable" : "Reviewable transcript"}
          </label>
          <textarea
            value={transcriptText}
            onChange={(event) => onTranscriptChange(event.target.value)}
            rows={6}
            className="w-full px-3 py-3 text-base border border-base-lighter rounded focus:outline-none focus:ring-2 focus:ring-primary resize-none"
            placeholder={lang === "es" ? "Edite la transcripción aquí..." : "Edit the transcript here..."}
            readOnly={voiceRecorderState === "forced_finalizing"}
          />
        </div>
      )}

      <div className="mb-5 flex items-start justify-center gap-6 sm:gap-10">
        <div className="flex flex-col items-center gap-2">
          <Button
            type="button"
            variant="secondary"
            className="!h-14 !w-14 !rounded-full !px-0 !py-0 shadow-sm"
            disabled={!canDiscardRecording}
            onClick={onDiscardVoiceRecording}
          >
            ×
          </Button>
          <span className="text-xs font-semibold text-primary-darkest">
            {voiceRecorderState === "review_ready" || voiceRecorderState === "audio_ready"
              ? (lang === "es" ? "Regrabar" : "Re-record")
              : (lang === "es" ? "Descartar" : "Discard")}
          </span>
        </div>

        <div className="flex flex-col items-center gap-2">
          <Button
            type="button"
            variant="danger"
            className="!h-16 !w-16 !rounded-full !px-0 !py-0 shadow-md"
            disabled={!canToggleRecording}
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
            disabled={!canCompleteRecording}
            onClick={onCompleteVoiceRecording}
          >
            ✓
          </Button>
          <span className="text-xs font-semibold text-primary-darkest">
            {lang === "es" ? "Complete" : "Complete"}
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
