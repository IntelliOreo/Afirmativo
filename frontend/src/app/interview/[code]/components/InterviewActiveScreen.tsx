"use client";

import { useCallback, useEffect } from "react";
import type { Lang } from "@/lib/language";
import { TEXT_ANSWER_MAX_CHARS } from "../constants";
import { useAnswerDraft } from "../hooks/useAnswerDraft";
import { useAnswerTimeout } from "../hooks/useAnswerTimeout";
import { useMicrophoneDialogState } from "../hooks/useMicrophoneDialogState";
import { useVoiceRecorder } from "../hooks/useVoiceRecorder";
import {
  getAnswerTimerMessage,
  getInterviewMessages,
  getVoiceFeedbackMessage,
} from "../messages/interviewMessages";
import type { Question } from "../models";
import { formatClock, getVoiceCapabilities } from "../utils";
import type { InputMode } from "../viewTypes";
import { InputModeSwitch } from "./InputModeSwitch";
import { MicrophoneWarmupDialog } from "./MicrophoneWarmupDialog";
import { TextAnswerPanel } from "./TextAnswerPanel";
import { TimeoutDialog } from "./TimeoutDialog";
import { VoiceAnswerSection } from "./VoiceAnswerSection";

interface InterviewActiveScreenProps {
  lang: Lang;
  code: string;
  phase: "active" | "submitting";
  currentQuestion: Question;
  textAnswer: string;
  inputMode: InputMode;
  answerSecondsLeft: number;
  isTimerExpired: boolean;
  hasMicOptIn: boolean;
  onMicOptIn: () => void;
  onTextChange: (value: string) => void;
  onInputModeChange: (mode: InputMode) => void;
  requestSubmit: (answerText: string) => void;
}

function answerTimerTone(answerSecondsLeft: number): "normal" | "warning" | "danger" {
  if (answerSecondsLeft <= 30) return "danger";
  if (answerSecondsLeft <= 60) return "warning";
  return "normal";
}

export function InterviewActiveScreen({
  lang,
  code,
  phase,
  currentQuestion,
  textAnswer,
  inputMode,
  answerSecondsLeft,
  isTimerExpired,
  hasMicOptIn,
  onMicOptIn,
  onTextChange,
  onInputModeChange,
  requestSubmit,
}: InterviewActiveScreenProps) {
  const shouldKeepMicWarm = hasMicOptIn && (phase === "active" || phase === "submitting");

  const {
    voiceRecorderState,
    micWarmState,
    voiceDurationSeconds,
    voiceWarningSeconds,
    voiceBlob,
    voicePreviewUrl,
    isVoicePreviewPlaying,
    voiceError,
    voiceInfo,
    isRecordingActive: voiceIsRecordingActive,
    isRecordingPaused: voiceIsRecordingPaused,
    prepareMicrophone,
    startVoiceRecording,
    completeVoiceRecording,
    discardVoiceRecording,
    toggleVoicePreviewPlayback,
    reviewVoiceRecording,
    setVoiceErrorFeedback,
  } = useVoiceRecorder({
    lang,
    isActive: phase === "active",
    shouldKeepMicWarm,
  });

  const {
    showMicrophoneDialog,
    activeMicDialogMode,
    micDialogUiState,
    handleEnableMicrophone,
    handleDismissMicrophonePrompt,
  } = useMicrophoneDialogState({
    micWarmState,
    prepareMicrophone,
    currentQuestion,
    phase,
    hasMicOptIn,
    onMicOptIn,
  });

  const handleReviewVoiceAnswer = useCallback(async () => {
    if (phase !== "active") return;
    const transcript = await reviewVoiceRecording(code);
    if (!transcript) return;
    onTextChange(transcript);
  }, [code, onTextChange, phase, reviewVoiceRecording]);

  const {
    shouldShowTimeoutDialog,
    isTimeoutDialogTranscribing,
    isTimeoutDialogInterrupted,
    handleTimeoutSubmit,
  } = useAnswerTimeout({
    phase,
    currentQuestion,
    isTimerExpired,
    answerSecondsLeft,
    inputMode,
    voiceRecorderState,
    textAnswer,
    completeVoiceRecording,
    onAutoReviewVoice: handleReviewVoiceAnswer,
    requestSubmit,
  });

  useAnswerDraft({
    code,
    currentQuestion,
    textAnswer,
    inputMode,
    voiceRecorderState,
    lang,
    phase,
    onTextChange,
    onInputModeChange,
  });

  // Discard voice recording on question change
  useEffect(() => {
    discardVoiceRecording();
  }, [currentQuestion.turnId, discardVoiceRecording]);

  const handleInputModeSwitch = useCallback((nextMode: InputMode) => {
    if (phase !== "active" || nextMode === inputMode) return;

    if (
      voiceRecorderState === "recording"
      || voiceRecorderState === "paused"
      || voiceRecorderState === "transcribing_for_review"
    ) {
      setVoiceErrorFeedback({ code: "switch_mode_while_recording" });
      return;
    }

    const hasUnreviewedVoice =
      inputMode === "voice"
      && !!voiceBlob
      && voiceRecorderState === "audio_ready";
    if (hasUnreviewedVoice && typeof window !== "undefined") {
      const confirmed = window.confirm(getInterviewMessages(lang).page.discardVoiceSwitchConfirm);
      if (!confirmed) return;
      discardVoiceRecording();
    }
    onInputModeChange(nextMode);
  }, [
    onInputModeChange,
    discardVoiceRecording,
    inputMode,
    lang,
    phase,
    setVoiceErrorFeedback,
    voiceBlob,
    voiceRecorderState,
  ]);

  const handleSubmit = useCallback(() => {
    if (phase !== "active" || !textAnswer.trim()) return;
    requestSubmit(textAnswer);
  }, [phase, requestSubmit, textAnswer]);

  const handleSelectTextInput = useCallback(() => {
    handleInputModeSwitch("text");
  }, [handleInputModeSwitch]);

  const handleSelectVoiceInput = useCallback(() => {
    handleInputModeSwitch("voice");
  }, [handleInputModeSwitch]);

  const canSwitchModes = getVoiceCapabilities({
    phase,
    voiceRecorderState,
    voiceBlob,
    voicePreviewUrl,
    hasDraftText: textAnswer.trim().length > 0,
    isFinalReviewWindow: answerSecondsLeft > 0 && answerSecondsLeft <= 30,
  }).canSwitchModes;
  const isVoiceMode = inputMode === "voice";
  const answerTimerLabel = formatClock(answerSecondsLeft);
  const textAnswerCharCount = textAnswer.length;
  const voiceErrorMessage = getVoiceFeedbackMessage(lang, voiceError);
  const voiceInfoMessage = getVoiceFeedbackMessage(lang, voiceInfo);

  return (
    <>
      {showMicrophoneDialog && activeMicDialogMode && (
        <MicrophoneWarmupDialog
          lang={lang}
          mode={activeMicDialogMode}
          uiState={micDialogUiState}
          onEnableMicrophone={handleEnableMicrophone}
          onDismiss={handleDismissMicrophonePrompt}
        />
      )}

      {shouldShowTimeoutDialog && (
        <TimeoutDialog
          lang={lang}
          isInterrupted={isTimeoutDialogInterrupted}
          isTranscribing={isTimeoutDialogTranscribing}
          onSubmit={handleTimeoutSubmit}
        />
      )}

      {phase === "active" && (
        <>
          <InputModeSwitch
            lang={lang}
            inputMode={inputMode}
            canSwitchModes={canSwitchModes && !isTimerExpired}
            onSelectText={handleSelectTextInput}
            onSelectVoice={handleSelectVoiceInput}
          />

          {!isVoiceMode ? (
            <TextAnswerPanel
              lang={lang}
              answerTimerLabel={answerTimerLabel}
              answerTimerTone={answerTimerTone(answerSecondsLeft)}
              answerTimerMessage={getAnswerTimerMessage(lang, answerSecondsLeft)}
              textAnswer={textAnswer}
              textAnswerCharCount={textAnswerCharCount}
              maxChars={TEXT_ANSWER_MAX_CHARS}
              isTimerExpired={isTimerExpired}
              onTextAnswerChange={onTextChange}
              onSubmitAnswer={handleSubmit}
            />
          ) : (
            <VoiceAnswerSection
              lang={lang}
              hasMicOptIn={hasMicOptIn}
              micWarmState={micWarmState}
              answerSecondsLeft={answerSecondsLeft}
              isTimerExpired={isTimerExpired}
              textAnswer={textAnswer}
              voiceRecorderState={voiceRecorderState}
              voiceDurationSeconds={voiceDurationSeconds}
              voiceWarningSeconds={voiceWarningSeconds}
              voiceBlob={voiceBlob}
              voicePreviewUrl={voicePreviewUrl}
              isVoicePreviewPlaying={isVoicePreviewPlaying}
              onToggleVoicePreviewPlayback={toggleVoicePreviewPlayback}
              voiceIsRecordingActive={voiceIsRecordingActive}
              voiceError={voiceErrorMessage}
              voiceInfo={voiceInfoMessage}
              voiceIsRecordingPaused={voiceIsRecordingPaused}
              onPrepareMicrophone={handleEnableMicrophone}
              onDiscardVoiceRecording={discardVoiceRecording}
              onStartVoiceRecording={startVoiceRecording}
              onReviewVoiceAnswer={handleReviewVoiceAnswer}
              onTranscriptChange={onTextChange}
              onSubmitAnswer={handleSubmit}
            />
          )}
        </>
      )}
    </>
  );
}
