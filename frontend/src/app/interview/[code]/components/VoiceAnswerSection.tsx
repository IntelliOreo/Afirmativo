"use client";

import { memo } from "react";
import type { Lang } from "@/lib/language";
import {
  VOICE_MAX_SECONDS,
} from "../constants";
import { getAnswerTimerMessage, getVoiceReviewWarning } from "../messages/interviewMessages";
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
  onReviewVoiceAnswer: () => Promise<void>;
  onTranscriptChange: (nextValue: string) => void;
  onSubmitAnswer: () => Promise<void> | void;
}

function answerTimerTone(answerSecondsLeft: number): "normal" | "warning" | "danger" {
  if (answerSecondsLeft <= 30) return "danger";
  if (answerSecondsLeft <= 60) return "warning";
  return "normal";
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
      ? getVoiceReviewWarning(lang)
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
      answerTimerMessage={getAnswerTimerMessage(lang, answerSecondsLeft)}
      voiceTimerLabel={voiceTimerLabel}
      canReplayRecording={voiceCaps.canReplayRecording}
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
