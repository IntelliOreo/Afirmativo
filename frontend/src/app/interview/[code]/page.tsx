"use client";

import { Suspense, useCallback, useEffect, useRef, useState } from "react";
import { useParams, useSearchParams } from "next/navigation";
import Link from "next/link";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import { beforeYouStartContent } from "../../../../content/beforeYouStart";
import { DisclaimerConsentPanel } from "./components/DisclaimerConsentPanel";
import { InterviewErrorState } from "./components/InterviewErrorState";
import { InterviewGuardState } from "./components/InterviewGuardState";
import { InterviewProgressHeader } from "./components/InterviewProgressHeader";
import { InterviewQuestionCard } from "./components/InterviewQuestionCard";
import { InputModeSwitch } from "./components/InputModeSwitch";
import { MicrophoneWarmupDialog } from "./components/MicrophoneWarmupDialog";
import { ReportSection } from "./components/ReportSection";
import { TextAnswerPanel } from "./components/TextAnswerPanel";
import { TimeoutDialog } from "./components/TimeoutDialog";
import { VoiceAnswerSection } from "./components/VoiceAnswerSection";
import {
  TEXT_ANSWER_MAX_CHARS,
  WARNING_AT_SECONDS,
  WRAPUP_AT_SECONDS,
} from "./constants";
import { useDisclaimerScrollGate } from "./hooks/useDisclaimerScrollGate";
import { useInterviewLanguage } from "./hooks/useInterviewLanguage";
import { useInterviewMachine } from "./hooks/useInterviewMachine";
import { useInterviewReport } from "./hooks/useInterviewReport";
import { useInterviewWaitingStatus } from "./hooks/useInterviewWaitingStatus";
import { useVoiceRecorder } from "./hooks/useVoiceRecorder";
import {
  getAnswerTimerMessage,
  getInterviewMessages,
  getReportErrorMessage,
  getVoiceFeedbackMessage,
} from "./messages/interviewMessages";
import type {
  InputMode,
  MicrophoneWarmupDialogMode,
  MicrophoneWarmupDialogState,
} from "./viewTypes";
import * as answerDraftStore from "@/lib/storage/answerDraftStore";
import {
  formatClock,
  getVoiceCapabilities,
  isReloadRecoveryErrorCode,
  parseDisclaimerBlocks,
} from "./utils";
import { getQuestionText } from "./models";

const MIC_RECONNECT_DIALOG_REVEAL_MS = 300;
const MIC_SUCCESS_HANDOFF_DELAY_MS = 500;

function InterviewPageContent() {
  const params = useParams();
  const searchParams = useSearchParams();
  const code = params.code as string;
  const requestedLang = searchParams.get("lang");

  const { lang, setLang, langInitialized } = useInterviewLanguage(code, requestedLang);
  const t = getInterviewMessages(lang);
  const {
    reportStatus,
    report,
    reportError,
    loadReport,
    resumeReport,
    printReport,
  } = useInterviewReport(code);
  const {
    state,
    dispatch,
    requestSubmit,
    retryPendingRecovery,
    canRetryPendingRecovery,
  } = useInterviewMachine({
    code,
    lang,
    langInitialized,
    setLang,
  });

  const currentQuestion =
    state.phase === "active" || state.phase === "submitting"
      ? state.question
      : null;
  const secondsLeft =
    state.phase === "active" || state.phase === "submitting"
      ? state.secondsLeft
      : 0;
  const answerSecondsLeft =
    state.phase === "active" || state.phase === "submitting"
      ? state.answerSecondsLeft
      : 0;
  const textAnswer = state.phase === "active" ? state.textAnswer : "";
  const inputMode = state.phase === "active" ? state.inputMode : "text";
  const submitMode = state.phase === "submitting" ? state.submitMode : null;
  const completionSource = state.phase === "done" ? state.completionSource : "finished";
  const error = state.phase === "error" ? state.message : "";
  const errorCode = state.phase === "error" ? state.code ?? "" : "";
  const [hasMicOptIn, setHasMicOptIn] = useState(false);
  const [hasSeenMicReadinessPrompt, setHasSeenMicReadinessPrompt] = useState(false);
  const [activeMicDialogMode, setActiveMicDialogMode] = useState<MicrophoneWarmupDialogMode | null>(null);
  const [micDialogUiState, setMicDialogUiState] = useState<MicrophoneWarmupDialogState>("idle");
  const [suppressReconnectDialog, setSuppressReconnectDialog] = useState(false);
  const [showTimeoutDialog, setShowTimeoutDialog] = useState(false);
  const restoredDraftTurnRef = useRef("");
  const timedOutTurnRef = useRef("");
  const timeoutReviewTurnRef = useRef("");
  const liveExpiredTurnRef = useRef("");
  const previousAnswerWindowRef = useRef<{ turnId: string; answerSecondsLeft: number } | null>(null);
  const reconnectDialogTimerRef = useRef<number | null>(null);
  const micSuccessHandoffTimerRef = useRef<number | null>(null);
  const shouldKeepMicWarm =
    hasMicOptIn
    && (state.phase === "active" || state.phase === "submitting");

  const {
    disclaimerScrollRef,
    hasReachedDisclaimerBottom,
    updateDisclaimerScrollState,
  } = useDisclaimerScrollGate(currentQuestion);

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
    isActive: state.phase === "active",
    shouldKeepMicWarm,
  });

  useEffect(() => {
    if (!currentQuestion?.turnId) return;
    restoredDraftTurnRef.current = "";
    discardVoiceRecording();
  }, [currentQuestion?.turnId, discardVoiceRecording]);

  useEffect(() => {
    if (!currentQuestion?.turnId) {
      setShowTimeoutDialog(false);
      timedOutTurnRef.current = "";
      timeoutReviewTurnRef.current = "";
      liveExpiredTurnRef.current = "";
      previousAnswerWindowRef.current = null;
      return;
    }

    setShowTimeoutDialog(false);
    timedOutTurnRef.current = "";
    timeoutReviewTurnRef.current = "";
    liveExpiredTurnRef.current = "";
    previousAnswerWindowRef.current = null;
  }, [currentQuestion?.turnId]);

  useEffect(() => {
    if (state.phase !== "active" || !currentQuestion) return;

    answerDraftStore.clearStale(code, currentQuestion.turnId);
    if (restoredDraftTurnRef.current === currentQuestion.turnId) return;
    restoredDraftTurnRef.current = currentQuestion.turnId;

    const savedDraft = answerDraftStore.read(code, currentQuestion.turnId);
    if (!savedDraft) return;

    dispatch({ type: "TEXT_CHANGED", payload: { value: savedDraft.draftText } });
    if (savedDraft.source === "voice_review" && currentQuestion.kind !== "disclaimer") {
      dispatch({ type: "INPUT_MODE_CHANGED", payload: { mode: "voice" } });
    }
  }, [code, currentQuestion, dispatch, state.phase]);

  useEffect(() => {
    if (state.phase !== "active" || !currentQuestion) return;

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
  }, [code, currentQuestion, inputMode, lang, state.phase, textAnswer, voiceRecorderState]);

  useEffect(() => {
    if (state.phase !== "done") return;
    void resumeReport();
  }, [resumeReport, state.phase]);

  const clearReconnectDialogTimer = useCallback(() => {
    if (reconnectDialogTimerRef.current == null) return;
    window.clearTimeout(reconnectDialogTimerRef.current);
    reconnectDialogTimerRef.current = null;
  }, []);

  const clearMicSuccessHandoffTimer = useCallback(() => {
    if (micSuccessHandoffTimerRef.current == null) return;
    window.clearTimeout(micSuccessHandoffTimerRef.current);
    micSuccessHandoffTimerRef.current = null;
  }, []);

  const closeMicrophoneDialog = useCallback(() => {
    clearReconnectDialogTimer();
    clearMicSuccessHandoffTimer();
    setActiveMicDialogMode(null);
    setMicDialogUiState("idle");
  }, [clearMicSuccessHandoffTimer, clearReconnectDialogTimer]);

  const beginMicSuccessHandoff = useCallback((mode: MicrophoneWarmupDialogMode) => {
    clearReconnectDialogTimer();
    clearMicSuccessHandoffTimer();
    setActiveMicDialogMode(mode);
    setMicDialogUiState("ready_handoff");
    micSuccessHandoffTimerRef.current = window.setTimeout(() => {
      if (mode === "initial_setup") {
        setHasMicOptIn(true);
        setHasSeenMicReadinessPrompt(true);
      }
      setSuppressReconnectDialog(false);
      setActiveMicDialogMode(null);
      setMicDialogUiState("idle");
      micSuccessHandoffTimerRef.current = null;
    }, MIC_SUCCESS_HANDOFF_DELAY_MS);
  }, [clearMicSuccessHandoffTimer, clearReconnectDialogTimer]);

  const handleInputModeSwitch = useCallback((nextMode: InputMode) => {
    if (state.phase !== "active" || nextMode === state.inputMode) return;

    if (
      voiceRecorderState === "recording"
      || voiceRecorderState === "paused"
      || voiceRecorderState === "transcribing_for_review"
    ) {
      setVoiceErrorFeedback({ code: "switch_mode_while_recording" });
      return;
    }

    const hasUnreviewedVoice =
      state.inputMode === "voice"
      && !!voiceBlob
      && voiceRecorderState === "audio_ready";
    if (hasUnreviewedVoice && typeof window !== "undefined") {
      const confirmed = window.confirm(t.page.discardVoiceSwitchConfirm);
      if (!confirmed) return;
      discardVoiceRecording();
    }
    dispatch({ type: "INPUT_MODE_CHANGED", payload: { mode: nextMode } });
  }, [
    dispatch,
    discardVoiceRecording,
    setVoiceErrorFeedback,
    state,
    voiceBlob,
    voiceRecorderState,
  ]);

  const handleSubmitAnswer = useCallback(() => {
    if (state.phase !== "active" || !state.textAnswer.trim()) return;
    requestSubmit(state.textAnswer);
  }, [requestSubmit, state]);

  const handleReviewVoiceAnswer = useCallback(async () => {
    if (state.phase !== "active") return;
    const transcript = await reviewVoiceRecording(code);
    if (!transcript) return;
    dispatch({ type: "TEXT_CHANGED", payload: { value: transcript } });
  }, [code, dispatch, reviewVoiceRecording, state.phase]);

  const handleSubmitReviewedVoiceAnswer = useCallback(() => {
    if (state.phase !== "active" || !state.textAnswer.trim()) return;
    requestSubmit(state.textAnswer);
  }, [requestSubmit, state]);

  const handleTimeoutSubmit = useCallback(() => {
    if (state.phase !== "active") return;
    requestSubmit(state.textAnswer);
  }, [requestSubmit, state]);

  const handleTextAnswerChange = useCallback((nextValue: string) => {
    dispatch({ type: "TEXT_CHANGED", payload: { value: nextValue } });
  }, [dispatch]);

  const handleEnableMicrophone = useCallback(async () => {
    const nextDialogMode = activeMicDialogMode ?? (hasMicOptIn ? "reconnect" : "initial_setup");
    clearReconnectDialogTimer();
    clearMicSuccessHandoffTimer();
    setSuppressReconnectDialog(false);
    setActiveMicDialogMode(nextDialogMode);
    setMicDialogUiState(micWarmState === "recovering" ? "recovering" : "warming");
    const prepared = await prepareMicrophone();
    if (!prepared) return;
    beginMicSuccessHandoff(nextDialogMode);
  }, [
    activeMicDialogMode,
    beginMicSuccessHandoff,
    clearMicSuccessHandoffTimer,
    clearReconnectDialogTimer,
    hasMicOptIn,
    micWarmState,
    prepareMicrophone,
  ]);

  const handleDismissMicrophonePrompt = useCallback(() => {
    if (activeMicDialogMode === "initial_setup") {
      setHasSeenMicReadinessPrompt(true);
    } else if (activeMicDialogMode === "reconnect") {
      setSuppressReconnectDialog(true);
    }
    closeMicrophoneDialog();
  }, [activeMicDialogMode, closeMicrophoneDialog]);

  const handleSelectTextInput = useCallback(() => {
    handleInputModeSwitch("text");
  }, [handleInputModeSwitch]);

  useEffect(() => {
    return () => {
      clearReconnectDialogTimer();
      clearMicSuccessHandoffTimer();
    };
  }, [clearMicSuccessHandoffTimer, clearReconnectDialogTimer]);

  useEffect(() => {
    if (activeMicDialogMode !== null || state.phase !== "active" || !currentQuestion) return;
    if (currentQuestion.kind !== "readiness" || hasMicOptIn || hasSeenMicReadinessPrompt) return;
    setActiveMicDialogMode("initial_setup");
    setMicDialogUiState("idle");
  }, [activeMicDialogMode, currentQuestion, hasMicOptIn, hasSeenMicReadinessPrompt, state.phase]);

  useEffect(() => {
    if (activeMicDialogMode === "initial_setup") {
      if (micDialogUiState === "ready_handoff") return;
      if (micWarmState === "warming") {
        setMicDialogUiState("warming");
      } else if (micWarmState === "recovering") {
        setMicDialogUiState("recovering");
      } else if (micWarmState === "denied") {
        setMicDialogUiState("denied");
      } else if (micWarmState === "error") {
        setMicDialogUiState("error");
      }
      return;
    }

    if (!hasMicOptIn || !(state.phase === "active" || state.phase === "submitting")) {
      clearReconnectDialogTimer();
      if (activeMicDialogMode === "reconnect") {
        closeMicrophoneDialog();
      }
      return;
    }

    if (micWarmState === "warm") {
      clearReconnectDialogTimer();
      if (
        activeMicDialogMode === "reconnect"
        && (micDialogUiState === "warming" || micDialogUiState === "recovering")
      ) {
        beginMicSuccessHandoff("reconnect");
      }
      setSuppressReconnectDialog(false);
      return;
    }

    if (micWarmState === "cold") {
      clearReconnectDialogTimer();
      setSuppressReconnectDialog(false);
      if (activeMicDialogMode === "reconnect") {
        closeMicrophoneDialog();
      }
      return;
    }

    if (micWarmState === "recovering") {
      if (activeMicDialogMode === "reconnect") {
        setMicDialogUiState("recovering");
        return;
      }
      if (suppressReconnectDialog) return;
      clearReconnectDialogTimer();
      reconnectDialogTimerRef.current = window.setTimeout(() => {
        setActiveMicDialogMode("reconnect");
        setMicDialogUiState("recovering");
        reconnectDialogTimerRef.current = null;
      }, MIC_RECONNECT_DIALOG_REVEAL_MS);
      return;
    }

    clearReconnectDialogTimer();
    if (micWarmState === "denied" || micWarmState === "error") {
      if (suppressReconnectDialog) return;
      setActiveMicDialogMode("reconnect");
      setMicDialogUiState(micWarmState);
    }
  }, [
    activeMicDialogMode,
    beginMicSuccessHandoff,
    clearReconnectDialogTimer,
    closeMicrophoneDialog,
    currentQuestion,
    hasMicOptIn,
    micDialogUiState,
    micWarmState,
    state.phase,
    suppressReconnectDialog,
  ]);

  const handleSelectVoiceInput = useCallback(() => {
    handleInputModeSwitch("voice");
  }, [handleInputModeSwitch]);

  const isTimerExpired = answerSecondsLeft <= 0
    && state.phase === "active"
    && currentQuestion?.kind === "criterion";

  useEffect(() => {
    if (state.phase !== "active" || !currentQuestion || currentQuestion.kind !== "criterion") {
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
  }, [answerSecondsLeft, currentQuestion, state.phase]);

  useEffect(() => {
    if (!isTimerExpired) return;
    if (voiceRecorderState === "recording" || voiceRecorderState === "paused") {
      completeVoiceRecording();
    }
  }, [isTimerExpired, voiceRecorderState, completeVoiceRecording]);

  useEffect(() => {
    if (state.phase !== "active" || !currentQuestion || currentQuestion.kind !== "criterion") {
      setShowTimeoutDialog(false);
      return;
    }
    if (!isTimerExpired) return;
    if (timedOutTurnRef.current === currentQuestion.turnId) return;
    timedOutTurnRef.current = currentQuestion.turnId;
    setShowTimeoutDialog(true);
  }, [currentQuestion, isTimerExpired, state.phase]);

  const handleAgreeAndContinue = useCallback(() => {
    if (state.phase !== "active") return;
    requestSubmit(t.page.understandAnswer);
  }, [requestSubmit, state.phase, t.page.understandAnswer]);

  const handleReloadPage = useCallback(() => {
    if (typeof window === "undefined") return;
    window.location.reload();
  }, []);

  const timerLabel = formatClock(secondsLeft);
  const answerTimerLabel = formatClock(answerSecondsLeft);
  const textAnswerCharCount = textAnswer.length;
  const isWarning = secondsLeft <= WARNING_AT_SECONDS;
  const isWrapup = secondsLeft <= WRAPUP_AT_SECONDS;
  const isBlinkingTimer = secondsLeft <= 30 && secondsLeft > 0;
  const isSubmittingInQuestionFlow = state.phase === "submitting" && state.submitMode === "question";
  const isConsentQuestion = currentQuestion?.kind === "disclaimer";
  const isReadinessQuestion = currentQuestion?.kind === "readiness";
  const consentQuestionText = getQuestionText(currentQuestion, lang);
  const consentBlocks = parseDisclaimerBlocks(consentQuestionText);
  const consentWarningAlert = beforeYouStartContent[lang].warningAlert;
  const progressPct = currentQuestion
    ? (currentQuestion.questionNumber / currentQuestion.totalQuestions) * 100
    : 0;
  const showInterviewProgress = state.phase === "active" || isSubmittingInQuestionFlow;
  const isReloadRecoveryError = isReloadRecoveryErrorCode(errorCode);
  const isVoiceMode = inputMode === "voice";
  const showMicrophoneDialog = activeMicDialogMode !== null;
  const answerTimerTone = answerSecondsLeft <= 30
    ? "danger"
    : answerSecondsLeft <= 60
      ? "warning"
      : "normal";
  const answerTimerMessage = getAnswerTimerMessage(lang, answerSecondsLeft);
  const reportErrorMessage = getReportErrorMessage(lang, reportError);
  const voiceErrorMessage = getVoiceFeedbackMessage(lang, voiceError);
  const voiceInfoMessage = getVoiceFeedbackMessage(lang, voiceInfo);
  const hasRecoverableVoiceState =
    voiceRecorderState === "audio_ready"
    || voiceRecorderState === "transcribing_for_review"
    || voiceRecorderState === "review_ready";
  const hasUsableLocalRecoveryPath =
    textAnswer.trim().length > 0
    || hasRecoverableVoiceState;
  const shouldShowTimeoutDialog =
    showTimeoutDialog
    && state.phase === "active"
    && currentQuestion?.kind === "criterion"
    && isTimerExpired;
  const shouldAutoReviewTimedOutVoice =
    shouldShowTimeoutDialog
    && isVoiceMode
    && voiceRecorderState === "audio_ready"
    && timeoutReviewTurnRef.current !== currentQuestion?.turnId;
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
    && currentQuestion?.turnId !== undefined
    && liveExpiredTurnRef.current !== currentQuestion?.turnId;
  const canSwitchModes = getVoiceCapabilities({
    phase: state.phase,
    voiceRecorderState,
    voiceBlob,
    voicePreviewUrl,
    hasDraftText: textAnswer.trim().length > 0,
    isFinalReviewWindow: answerSecondsLeft > 0 && answerSecondsLeft <= 30,
  }).canSwitchModes;
  const {
    startupWaitStatus,
    questionWaitStatus,
    reportWaitStatus,
  } = useInterviewWaitingStatus({
    lang,
    phase: state.phase,
    submitMode,
    reportStatus,
  });

  useEffect(() => {
    if (!shouldAutoReviewTimedOutVoice || !currentQuestion?.turnId) return;
    timeoutReviewTurnRef.current = currentQuestion.turnId;
    void handleReviewVoiceAnswer();
  }, [currentQuestion?.turnId, handleReviewVoiceAnswer, shouldAutoReviewTimedOutVoice]);

  return (
    <div className="interview-page flex flex-col min-h-screen">
      <NavHeader lang={lang} />

      {showInterviewProgress && (
        <InterviewProgressHeader
          lang={lang}
          isBlinkingTimer={isBlinkingTimer}
          isWrapup={isWrapup}
          isWarning={isWarning}
          timerLabel={timerLabel}
          question={currentQuestion}
          progressPct={progressPct}
        />
      )}

      <main className="flex-1 bg-base-lightest">
        <div className="max-w-2xl mx-auto px-4 py-8">
          {state.phase === "guard" && (
            <InterviewGuardState lang={lang} code={code} />
          )}

          {state.phase === "loading" && (
            <Card className="text-center py-8">
              <p className="text-primary-darkest mb-3">
                {t.page.loading}
              </p>
              {startupWaitStatus && (
                <p className="text-base sm:text-lg text-primary-dark leading-snug">{startupWaitStatus}</p>
              )}
            </Card>
          )}

          {state.phase === "error" && (
            <InterviewErrorState
              lang={lang}
              code={code}
              error={error}
              isReloadRecoveryError={isReloadRecoveryError}
              canRetryPendingAnswer={canRetryPendingRecovery}
              onRetryPendingAnswer={retryPendingRecovery}
              onReloadPage={handleReloadPage}
            />
          )}

          {state.phase === "done" && (
            <>
              <ReportSection
                completionSource={completionSource}
                report={report}
                reportError={reportErrorMessage}
                reportStatus={reportStatus}
                reportWaitStatus={reportWaitStatus}
                lang={lang}
                onLoadReport={loadReport}
                onCheckAgain={resumeReport}
                onPrintReport={printReport}
              />

              <Link href="/">
                <Button fullWidth variant="secondary">
                  {t.page.backHome}
                </Button>
              </Link>
            </>
          )}

          {(state.phase === "active" || isSubmittingInQuestionFlow) && currentQuestion && (
            <>
              {!isConsentQuestion && (
                <InterviewQuestionCard
                  questionText={lang === "es" ? currentQuestion.textEs : currentQuestion.textEn}
                />
              )}

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

              {isSubmittingInQuestionFlow ? (
                <Card className="mb-6 text-center py-10 px-4">
                  <p className="text-xs font-semibold uppercase tracking-wider text-primary">
                    {t.page.processingAnswer}
                  </p>
                  {questionWaitStatus && (
                    <p className="mt-3 text-base sm:text-lg text-primary-dark leading-snug">
                      {questionWaitStatus}
                    </p>
                  )}
                  <div className="mt-6 inline-block h-8 w-8 border-4 border-primary border-t-transparent rounded-full animate-spin" />
                </Card>
              ) : isConsentQuestion ? (
                <DisclaimerConsentPanel
                  lang={lang}
                  consentBlocks={consentBlocks}
                  warningAlertText={consentWarningAlert}
                  hasReachedDisclaimerBottom={hasReachedDisclaimerBottom}
                  disclaimerScrollRef={disclaimerScrollRef}
                  onDisclaimerScroll={updateDisclaimerScrollState}
                  onAgreeAndContinue={handleAgreeAndContinue}
                />
              ) : (
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
                      answerTimerTone={answerTimerTone}
                      answerTimerMessage={answerTimerMessage}
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
          )}
        </div>
      </main>

      <Footer />
    </div>
  );
}

export default function InterviewPage() {
  return (
    <Suspense fallback={null}>
      <InterviewPageContent />
    </Suspense>
  );
}
