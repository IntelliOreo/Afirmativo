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
import { InterviewFinalSubmitState } from "./components/InterviewFinalSubmitState";
import { InterviewGuardState } from "./components/InterviewGuardState";
import { InterviewProgressHeader } from "./components/InterviewProgressHeader";
import { ReportSection } from "./components/ReportSection";
import { TextAnswerPanel } from "./components/TextAnswerPanel";
import { VoiceRecorderPanel } from "./components/VoiceRecorderPanel";
import {
  TEXT_ANSWER_MAX_CHARS,
  VOICE_MAX_SECONDS,
  WARNING_AT_SECONDS,
  WRAPUP_AT_SECONDS,
} from "./constants";
import { useDisclaimerScrollGate } from "./hooks/useDisclaimerScrollGate";
import { useInterviewLanguage } from "./hooks/useInterviewLanguage";
import { useInterviewMachine } from "./hooks/useInterviewMachine";
import { useInterviewReport } from "./hooks/useInterviewReport";
import { useInterviewWaitingStatus } from "./hooks/useInterviewWaitingStatus";
import { useVoiceRecorder } from "./hooks/useVoiceRecorder";
import type { InputMode } from "./viewTypes";
import * as answerDraftStore from "@/lib/storage/answerDraftStore";
import {
  formatClock,
  getVoiceCapabilities,
  isReloadRecoveryErrorCode,
  parseDisclaimerBlocks,
} from "./utils";
import { getQuestionText } from "./models";

function InterviewPageContent() {
  const params = useParams();
  const searchParams = useSearchParams();
  const code = params.code as string;
  const requestedLang = searchParams.get("lang");

  const { lang, setLang, langInitialized } = useInterviewLanguage(code, requestedLang);
  const {
    reportStatus,
    report,
    reportError,
    loadReport,
    printReport,
  } = useInterviewReport(code);
  const {
    state,
    dispatch,
    requestSubmit,
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
  const [forceFinalizingVoice, setForceFinalizingVoice] = useState(false);
  const restoredDraftTurnRef = useRef("");
  const autoFinalizeTurnRef = useRef("");

  const {
    disclaimerScrollRef,
    hasReachedDisclaimerBottom,
    updateDisclaimerScrollState,
  } = useDisclaimerScrollGate(currentQuestion);

  const {
    voiceRecorderState,
    voiceDurationSeconds,
    voiceWarningSeconds,
    voiceBlob,
    voicePreviewUrl,
    isVoicePreviewPlaying,
    voiceError,
    voiceInfo,
    isRecordingActive: voiceIsRecordingActive,
    isRecordingPaused: voiceIsRecordingPaused,
    startVoiceRecording,
    completeVoiceRecording,
    discardVoiceRecording,
    toggleVoicePreviewPlayback,
    reviewVoiceRecording,
    finalizeVoiceRecording,
    setVoiceErrorMessage,
  } = useVoiceRecorder({
    lang,
    isActive: state.phase === "active" || forceFinalizingVoice,
  });

  useEffect(() => {
    if (!currentQuestion?.turnId) return;
    restoredDraftTurnRef.current = "";
    autoFinalizeTurnRef.current = "";
    setForceFinalizingVoice(false);
    discardVoiceRecording();
  }, [currentQuestion?.turnId, discardVoiceRecording]);

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

  const handleInputModeSwitch = useCallback((nextMode: InputMode) => {
    if (state.phase !== "active" || nextMode === state.inputMode) return;

    if (
      voiceRecorderState === "recording"
      || voiceRecorderState === "paused"
      || voiceRecorderState === "transcribing_for_review"
      || voiceRecorderState === "forced_finalizing"
    ) {
      setVoiceErrorMessage(
        lang === "es"
          ? "Detenga la grabación antes de cambiar de modo."
          : "Stop recording before switching modes.",
      );
      return;
    }

    const hasUnreviewedVoice =
      state.inputMode === "voice"
      && !!voiceBlob
      && voiceRecorderState === "audio_ready";
    if (hasUnreviewedVoice && typeof window !== "undefined") {
      const confirmed = window.confirm(
        lang === "es"
          ? "Tiene audio sin revisar. ¿Desea descartarlo y cambiar de modo?"
          : "You have audio that has not been reviewed yet. Discard it and switch modes?",
      );
      if (!confirmed) return;
      discardVoiceRecording();
    }
    dispatch({ type: "INPUT_MODE_CHANGED", payload: { mode: nextMode } });
  }, [
    dispatch,
    discardVoiceRecording,
    lang,
    setVoiceErrorMessage,
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

  useEffect(() => {
    if (state.phase !== "active" || !currentQuestion) return;
    if (answerSecondsLeft > 0) return;
    if (autoFinalizeTurnRef.current === currentQuestion.turnId) return;
    autoFinalizeTurnRef.current = currentQuestion.turnId;

    void (async () => {
      const draftText = state.textAnswer.trim();
      if (draftText) {
        requestSubmit(draftText, "finalAuto");
        return;
      }

      const hasVoiceInProgress =
        voiceRecorderState === "recording"
        || voiceRecorderState === "paused"
        || voiceRecorderState === "audio_ready"
        || voiceRecorderState === "review_ready";

      if (!hasVoiceInProgress) {
        requestSubmit("", "finalAuto");
        return;
      }

      setForceFinalizingVoice(true);
      const transcript = await finalizeVoiceRecording(code);
      requestSubmit((transcript ?? "").trim(), "finalAuto");
    })();
  }, [
    answerSecondsLeft,
    code,
    currentQuestion,
    finalizeVoiceRecording,
    requestSubmit,
    state,
    voiceRecorderState,
  ]);

  const handleAgreeAndContinue = useCallback(() => {
    if (state.phase !== "active") return;
    requestSubmit(lang === "es" ? "Entiendo" : "I understand");
  }, [lang, requestSubmit, state.phase]);

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
  const consentQuestionText = getQuestionText(currentQuestion, lang);
  const consentBlocks = parseDisclaimerBlocks(consentQuestionText);
  const consentWarningAlert = beforeYouStartContent[lang].warningAlert;
  const progressPct = currentQuestion
    ? (currentQuestion.questionNumber / currentQuestion.totalQuestions) * 100
    : 0;
  const showInterviewProgress = state.phase === "active" || isSubmittingInQuestionFlow;
  const isReloadRecoveryError = isReloadRecoveryErrorCode(errorCode);
  const isVoiceMode = state.phase === "active" && inputMode === "voice" && !isConsentQuestion;
  const voiceTimerLabel = formatClock(voiceDurationSeconds);
  const voiceProgressPct = Math.min(100, (voiceDurationSeconds / VOICE_MAX_SECONDS) * 100);
  const voiceWarningRemaining = voiceWarningSeconds == null
    ? null
    : Math.max(0, VOICE_MAX_SECONDS - voiceWarningSeconds);
  const isAnswerFinalReviewWindow = answerSecondsLeft > 0 && answerSecondsLeft <= 30;
  const answerTimerTone = answerSecondsLeft <= 30
    ? "danger"
    : answerSecondsLeft <= 60
      ? "warning"
      : "normal";
  const answerTimerMessage = answerSecondsLeft <= 30
    ? (lang === "es"
      ? "Quedan 0:30 o menos. Su respuesta se finalizará automáticamente cuando llegue a 0:00."
      : "0:30 or less remain. Your answer will be finalized automatically when the timer reaches 0:00.")
    : answerSecondsLeft <= 60
      ? (lang === "es"
        ? "Queda 1:00 o menos. Termine y envíe su respuesta."
        : "1:00 or less remain. Finish and submit your answer.")
      : (lang === "es"
        ? "Use este tiempo para revisar y enviar su respuesta final."
        : "Use this time to review and submit your final answer.");
  const voiceReviewWarning =
    (voiceRecorderState === "recording" || voiceRecorderState === "paused") && answerSecondsLeft <= 60
      ? (lang === "es"
        ? "Deténgase pronto para dejar tiempo para revisar antes de enviar."
        : "Stop soon to leave time to review before submit.")
      : "";
  const voiceCaps = getVoiceCapabilities({
    phase: state.phase,
    voiceRecorderState,
    voiceBlob,
    voicePreviewUrl,
    hasDraftText: textAnswer.trim().length > 0,
    isFinalReviewWindow: isAnswerFinalReviewWindow,
  });
  const {
    startupWaitStatus,
    questionWaitStatus,
    finalSubmitWaitStatus,
    reportWaitStatus,
  } = useInterviewWaitingStatus({
    lang,
    phase: state.phase,
    submitMode,
    reportStatus,
  });

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
                {lang === "es" ? "Cargando..." : "Loading..."}
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
              onReloadPage={handleReloadPage}
            />
          )}

          {state.phase === "done" && (
            <>
              <ReportSection
                completionSource={completionSource}
                report={report}
                reportError={reportError}
                reportStatus={reportStatus}
                reportWaitStatus={reportWaitStatus}
                lang={lang}
                onLoadReport={loadReport}
                onPrintReport={printReport}
              />

              <Link href="/">
                <Button fullWidth variant="secondary">
                  {lang === "es" ? "Volver al inicio" : "Back to home"}
                </Button>
              </Link>
            </>
          )}

          {(forceFinalizingVoice || (state.phase === "submitting" && state.submitMode === "finalAuto")) && (
            <InterviewFinalSubmitState
              lang={lang}
              finalSubmitWaitStatus={finalSubmitWaitStatus}
            />
          )}

          {(state.phase === "active" || isSubmittingInQuestionFlow) && currentQuestion && !forceFinalizingVoice && (
            <>
              {!isConsentQuestion && (
                <Card className="mb-6">
                  <p className="text-lg font-semibold text-primary-dark whitespace-pre-line">
                    {lang === "es" ? currentQuestion.textEs : currentQuestion.textEn}
                  </p>
                </Card>
              )}

              {isSubmittingInQuestionFlow ? (
                <Card className="mb-6 text-center py-10 px-4">
                  <p className="text-xs font-semibold uppercase tracking-wider text-primary">
                    {lang === "es" ? "Procesando respuesta" : "Processing answer"}
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
                  <div className="mb-6 grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <Button
                      type="button"
                      variant={inputMode === "text" ? "primary" : "secondary"}
                      disabled={!voiceCaps.canSwitchModes}
                      onClick={() => handleInputModeSwitch("text")}
                    >
                      {lang === "es" ? "Entrada por texto" : "Text input"}
                    </Button>
                    <Button
                      type="button"
                      variant={inputMode === "voice" ? "primary" : "secondary"}
                      disabled={!voiceCaps.canSwitchModes}
                      onClick={() => handleInputModeSwitch("voice")}
                    >
                      {lang === "es" ? "Entrada por voz" : "Voice input"}
                    </Button>
                  </div>

                  {!isVoiceMode ? (
                    <TextAnswerPanel
                      lang={lang}
                      answerTimerLabel={answerTimerLabel}
                      answerTimerTone={answerTimerTone}
                      answerTimerMessage={answerTimerMessage}
                      textAnswer={textAnswer}
                      textAnswerCharCount={textAnswerCharCount}
                      maxChars={TEXT_ANSWER_MAX_CHARS}
                      onTextAnswerChange={(nextValue) => {
                        dispatch({ type: "TEXT_CHANGED", payload: { value: nextValue } });
                      }}
                      onSubmitAnswer={handleSubmitAnswer}
                    />
                  ) : (
                    <VoiceRecorderPanel
                      lang={lang}
                      answerTimerLabel={answerTimerLabel}
                      answerTimerTone={answerTimerTone}
                      answerTimerMessage={answerTimerMessage}
                      voiceTimerLabel={voiceTimerLabel}
                      canPreviewRecording={voiceCaps.canPreviewRecording}
                      isVoicePreviewPlaying={isVoicePreviewPlaying}
                      onToggleVoicePreviewPlayback={toggleVoicePreviewPlayback}
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
                      onDiscardVoiceRecording={discardVoiceRecording}
                      canToggleRecording={voiceCaps.canToggleRecording}
                      onStartVoiceRecording={startVoiceRecording}
                      centerControlLabel={voiceCaps.centerControlLabel}
                      canCompleteRecording={voiceCaps.canCompleteRecording}
                      onCompleteVoiceRecording={completeVoiceRecording}
                      canReviewTranscript={voiceCaps.canReviewTranscript}
                      onReviewVoiceAnswer={handleReviewVoiceAnswer}
                      canSubmitAnswer={voiceCaps.canSubmitAnswer}
                      transcriptText={textAnswer}
                      onTranscriptChange={(nextValue) => {
                        dispatch({ type: "TEXT_CHANGED", payload: { value: nextValue } });
                      }}
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
