"use client";

import { memo } from "react";
import type { Lang } from "@/lib/language";
import {
  VOICE_MAX_SECONDS,
} from "../constants";
import { formatClock, getVoiceCapabilities } from "../utils";
import type { MicWarmState, VoiceRecorderState } from "../viewTypes";
import { VoiceRecorderPanel } from "./VoiceRecorderPanel";

interface VoiceAnswerSectionProps {
  lang: Lang;
  hasMicOptIn: boolean;
  micWarmState: MicWarmState;
  answerSecondsLeft: number;
  isTimerExpired?: boolean;
  textAnswer: string;
  voiceRecorderState: VoiceRecorderState;
  voiceDurationSeconds: number;
  voiceWarningSeconds: number | null;
  voiceBlob: Blob | null;
  voicePreviewUrl: string | null;
  isVoicePreviewPlaying: boolean;
  voiceError: string;
  voiceInfo: string;
  voiceIsRecordingActive: boolean;
  voiceIsRecordingPaused: boolean;
  onPrepareMicrophone: () => Promise<void>;
  onToggleVoicePreviewPlayback: () => Promise<void>;
  onDiscardVoiceRecording: () => void;
  onStartVoiceRecording: () => Promise<void>;
  onCompleteVoiceRecording: () => void;
  onReviewVoiceAnswer: () => Promise<void>;
  onTranscriptChange: (nextValue: string) => void;
  onSubmitAnswer: () => Promise<void> | void;
}

function answerTimerTone(answerSecondsLeft: number): "normal" | "warning" | "danger" {
  if (answerSecondsLeft <= 30) return "danger";
  if (answerSecondsLeft <= 60) return "warning";
  return "normal";
}

function answerTimerMessage(lang: Lang, answerSecondsLeft: number): string {
  if (answerSecondsLeft <= 30) {
    return lang === "es"
      ? "Quedan 0:30 o menos. Termine y envíe su respuesta."
      : "0:30 or less remain. Finish and submit your answer.";
  }
  if (answerSecondsLeft <= 60) {
    return lang === "es"
      ? "Queda 1:00 o menos. Termine y envíe su respuesta."
      : "1:00 or less remain. Finish and submit your answer.";
  }
  return lang === "es"
    ? "Use este tiempo para revisar y enviar su respuesta final."
    : "Use this time to review and submit your final answer.";
}

export const VoiceAnswerSection = memo(function VoiceAnswerSection({
  lang,
  hasMicOptIn,
  micWarmState,
  answerSecondsLeft,
  isTimerExpired = false,
  textAnswer,
  voiceRecorderState,
  voiceDurationSeconds,
  voiceWarningSeconds,
  voiceBlob,
  voicePreviewUrl,
  isVoicePreviewPlaying,
  voiceError,
  voiceInfo,
  voiceIsRecordingActive,
  voiceIsRecordingPaused,
  onPrepareMicrophone,
  onToggleVoicePreviewPlayback,
  onDiscardVoiceRecording,
  onStartVoiceRecording,
  onCompleteVoiceRecording,
  onReviewVoiceAnswer,
  onTranscriptChange,
  onSubmitAnswer,
}: VoiceAnswerSectionProps) {
  const isAnswerFinalReviewWindow = answerSecondsLeft > 0 && answerSecondsLeft <= 30;
  const voiceTimerLabel = formatClock(voiceDurationSeconds);
  const voiceProgressPct = Math.min(100, (voiceDurationSeconds / VOICE_MAX_SECONDS) * 100);
  const voiceWarningRemaining = voiceWarningSeconds == null
    ? null
    : Math.max(0, VOICE_MAX_SECONDS - voiceWarningSeconds);
  const voiceReviewWarning =
    (voiceRecorderState === "recording" || voiceRecorderState === "paused") && answerSecondsLeft <= 60
      ? (lang === "es"
        ? "Deténgase pronto para dejar tiempo para revisar antes de enviar."
        : "Stop soon to leave time to review before submit.")
      : "";
  const voiceCaps = getVoiceCapabilities({
    phase: "active",
    voiceRecorderState,
    voiceBlob,
    voicePreviewUrl,
    hasDraftText: textAnswer.trim().length > 0,
    isFinalReviewWindow: isAnswerFinalReviewWindow,
  });

  return (
    <VoiceRecorderPanel
      lang={lang}
      hasMicOptIn={hasMicOptIn}
      micWarmState={micWarmState}
      onPrepareMicrophone={onPrepareMicrophone}
      answerTimerLabel={formatClock(answerSecondsLeft)}
      answerTimerTone={answerTimerTone(answerSecondsLeft)}
      answerTimerMessage={answerTimerMessage(lang, answerSecondsLeft)}
      voiceTimerLabel={voiceTimerLabel}
      canPreviewRecording={voiceCaps.canPreviewRecording}
      isVoicePreviewPlaying={isVoicePreviewPlaying}
      onToggleVoicePreviewPlayback={onToggleVoicePreviewPlayback}
      voiceIsRecordingActive={voiceIsRecordingActive}
      voiceProgressPct={voiceProgressPct}
      voiceWarningRemaining={voiceWarningRemaining}
      voiceReviewWarning={voiceReviewWarning}
      voiceError={voiceError}
      voiceInfo={voiceInfo}
      voiceRecorderState={voiceRecorderState}
      voiceIsRecordingPaused={voiceIsRecordingPaused}
      voiceBlob={voiceBlob}
      canDiscardRecording={voiceCaps.canDiscardRecording}
      onDiscardVoiceRecording={onDiscardVoiceRecording}
      canToggleRecording={voiceCaps.canToggleRecording}
      onStartVoiceRecording={onStartVoiceRecording}
      centerControlLabel={voiceCaps.centerControlLabel}
      canCompleteRecording={voiceCaps.canCompleteRecording}
      onCompleteVoiceRecording={onCompleteVoiceRecording}
      canReviewTranscript={voiceCaps.canReviewTranscript}
      onReviewVoiceAnswer={onReviewVoiceAnswer}
      canSubmitAnswer={voiceCaps.canSubmitAnswer}
      isTimerExpired={isTimerExpired}
      transcriptText={textAnswer}
      onTranscriptChange={onTranscriptChange}
      onSubmitAnswer={onSubmitAnswer}
    />
  );
});
