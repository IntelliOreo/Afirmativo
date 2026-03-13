"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { Dispatch } from "react";
import type { Lang } from "@/lib/language";
import * as answerDraftStore from "@/lib/storage/answerDraftStore";
import { TEXT_ANSWER_MAX_CHARS } from "../constants";
import { useMicrophoneDialogState } from "../hooks/useMicrophoneDialogState";
import { useVoiceRecorder } from "../hooks/useVoiceRecorder";
import { getTimeoutDialogState } from "../lib/getTimeoutDialogState";
import {
  getAnswerTimerMessage,
  getInterviewMessages,
  getVoiceFeedbackMessage,
} from "../messages/interviewMessages";
import type { Question } from "../models";
import { getQuestionText } from "../models";
import type { InterviewAction } from "../hooks/useInterviewMachine";
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
  secondsLeft: number;
  dispatch: Dispatch<InterviewAction>;
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
  secondsLeft: _secondsLeft,
  dispatch,
  requestSubmit,
}: InterviewActiveScreenProps) {
  const [hasMicOptIn, setHasMicOptIn] = useState(false);
  const [showTimeoutDialog, setShowTimeoutDialog] = useState(false);
  const restoredDraftTurnRef = useRef("");
  const timedOutTurnRef = useRef("");
  const timeoutReviewTurnRef = useRef("");
  const liveExpiredTurnRef = useRef("");
  const previousAnswerWindowRef = useRef<{ turnId: string; answerSecondsLeft: number } | null>(null);
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
    onMicOptIn: () => {
      setHasMicOptIn(true);
    },
  });

  useEffect(() => {
    restoredDraftTurnRef.current = "";
    discardVoiceRecording();
  }, [currentQuestion.turnId, discardVoiceRecording]);

  useEffect(() => {
    setShowTimeoutDialog(false);
    timedOutTurnRef.current = "";
    timeoutReviewTurnRef.current = "";
    liveExpiredTurnRef.current = "";
    previousAnswerWindowRef.current = null;
  }, [currentQuestion.turnId]);

  useEffect(() => {
    if (phase !== "active") return;

    answerDraftStore.clearStale(code, currentQuestion.turnId);
    if (restoredDraftTurnRef.current === currentQuestion.turnId) return;
    restoredDraftTurnRef.current = currentQuestion.turnId;

    const savedDraft = answerDraftStore.read(code, currentQuestion.turnId);
    if (!savedDraft) return;

    dispatch({ type: "TEXT_CHANGED", payload: { value: savedDraft.draftText } });
    if (savedDraft.source === "voice_review") {
      dispatch({ type: "INPUT_MODE_CHANGED", payload: { mode: "voice" } });
    }
  }, [code, currentQuestion.turnId, dispatch, phase]);

  useEffect(() => {
    if (phase !== "active") return;

    if (!textAnswer.trim()) {
      answerDraftStore.clear(code, currentQuestion.turnId);
      return;
    }

    answerDraftStore.write(code, {
      turnId: currentQuestion.turnId,
      questionText: getQuestionText(currentQuestion, lang),
      draftText: textAnswer,
      source: inputMode === "voice" && voiceRecorderState === "review_ready"
        ? "voice_review"
        : "text",
      updatedAt: Date.now(),
    });
  }, [code, currentQuestion, inputMode, lang, phase, textAnswer, voiceRecorderState]);

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
    dispatch({ type: "INPUT_MODE_CHANGED", payload: { mode: nextMode } });
  }, [
    dispatch,
    discardVoiceRecording,
    inputMode,
    lang,
    phase,
    setVoiceErrorFeedback,
    voiceBlob,
    voiceRecorderState,
  ]);

  const handleSubmitAnswer = useCallback(() => {
    if (phase !== "active" || !textAnswer.trim()) return;
    requestSubmit(textAnswer);
  }, [phase, requestSubmit, textAnswer]);

  const handleReviewVoiceAnswer = useCallback(async () => {
    if (phase !== "active") return;
    const transcript = await reviewVoiceRecording(code);
    if (!transcript) return;
    dispatch({ type: "TEXT_CHANGED", payload: { value: transcript } });
  }, [code, dispatch, phase, reviewVoiceRecording]);

  const handleSubmitReviewedVoiceAnswer = useCallback(() => {
    if (phase !== "active" || !textAnswer.trim()) return;
    requestSubmit(textAnswer);
  }, [phase, requestSubmit, textAnswer]);

  const handleTimeoutSubmit = useCallback(() => {
    if (phase !== "active") return;
    requestSubmit(textAnswer);
  }, [phase, requestSubmit, textAnswer]);

  const handleTextAnswerChange = useCallback((nextValue: string) => {
    dispatch({ type: "TEXT_CHANGED", payload: { value: nextValue } });
  }, [dispatch]);

  const handleSelectTextInput = useCallback(() => {
    handleInputModeSwitch("text");
  }, [handleInputModeSwitch]);

  const handleSelectVoiceInput = useCallback(() => {
    handleInputModeSwitch("voice");
  }, [handleInputModeSwitch]);

  useEffect(() => {
    if (phase !== "active" || currentQuestion.kind !== "criterion") {
      previousAnswerWindowRef.current = null;
      return;
    }

    const previous = previousAnswerWindowRef.current;
    if (
      previous
      && previous.turnId === currentQuestion.turnId
      && previous.answerSecondsLeft > 0
      && answerSecondsLeft <= 0
    ) {
      liveExpiredTurnRef.current = currentQuestion.turnId;
    }

    previousAnswerWindowRef.current = {
      turnId: currentQuestion.turnId,
      answerSecondsLeft,
    };
  }, [answerSecondsLeft, currentQuestion, phase]);

  useEffect(() => {
    if (!isTimerExpired) return;
    if (voiceRecorderState === "recording" || voiceRecorderState === "paused") {
      completeVoiceRecording();
    }
  }, [completeVoiceRecording, isTimerExpired, voiceRecorderState]);

  useEffect(() => {
    if (phase !== "active" || currentQuestion.kind !== "criterion") {
      setShowTimeoutDialog(false);
      return;
    }
    if (!isTimerExpired) return;
    if (timedOutTurnRef.current === currentQuestion.turnId) return;
    timedOutTurnRef.current = currentQuestion.turnId;
    setShowTimeoutDialog(true);
  }, [currentQuestion, isTimerExpired, phase]);

  const {
    shouldShowTimeoutDialog,
    shouldAutoReviewTimedOutVoice,
    isTimeoutDialogTranscribing,
    isTimeoutDialogInterrupted,
  } = getTimeoutDialogState({
    showTimeoutDialog,
    phase,
    currentQuestion,
    isTimerExpired,
    inputMode,
    voiceRecorderState,
    textAnswer,
    liveExpiredTurnId: liveExpiredTurnRef.current,
    timeoutReviewTurnId: timeoutReviewTurnRef.current,
  });

  useEffect(() => {
    if (!shouldAutoReviewTimedOutVoice) return;
    timeoutReviewTurnRef.current = currentQuestion.turnId;
    void handleReviewVoiceAnswer();
  }, [currentQuestion.turnId, handleReviewVoiceAnswer, shouldAutoReviewTimedOutVoice]);

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
              onTextAnswerChange={handleTextAnswerChange}
              onSubmitAnswer={handleSubmitAnswer}
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
              onCompleteVoiceRecording={completeVoiceRecording}
              onReviewVoiceAnswer={handleReviewVoiceAnswer}
              onTranscriptChange={handleTextAnswerChange}
              onSubmitAnswer={handleSubmitReviewedVoiceAnswer}
            />
          )}
        </>
      )}
    </>
  );
}
