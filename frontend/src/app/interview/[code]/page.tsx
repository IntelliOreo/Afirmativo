"use client";

import { Suspense, useCallback, useEffect } from "react";
import { useParams, useSearchParams } from "next/navigation";
import Link from "next/link";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import { Alert } from "@components/Alert";
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
  AUTOSUBMIT_SECONDS,
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
  const textAnswer = state.phase === "active" ? state.textAnswer : "";
  const inputMode = state.phase === "active" ? state.inputMode : "text";
  const submitMode = state.phase === "submitting" ? state.submitMode : null;
  const completionSource = state.phase === "done" ? state.completionSource : "finished";
  const error = state.phase === "error" ? state.message : "";
  const errorCode = state.phase === "error" ? state.code ?? "" : "";

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
    sendVoiceRecording,
    setVoiceErrorMessage,
  } = useVoiceRecorder({
    lang,
    isActive: state.phase === "active",
  });

  useEffect(() => {
    if (!currentQuestion?.turnId) return;
    discardVoiceRecording();
  }, [currentQuestion?.turnId, discardVoiceRecording]);

  const handleInputModeSwitch = useCallback((nextMode: InputMode) => {
    if (state.phase !== "active" || nextMode === state.inputMode) return;

    if (
      voiceRecorderState === "recording"
      || voiceRecorderState === "paused"
      || voiceRecorderState === "sending"
    ) {
      setVoiceErrorMessage(
        lang === "es"
          ? "Detenga la grabación antes de cambiar de modo."
          : "Stop recording before switching modes.",
      );
      return;
    }

    const hasUnsentText = state.inputMode === "text" && state.textAnswer.trim().length > 0;
    const hasUnsentVoice = state.inputMode === "voice" && !!voiceBlob;
    if ((hasUnsentText || hasUnsentVoice) && typeof window !== "undefined") {
      const confirmed = window.confirm(
        lang === "es"
          ? "Tiene una respuesta sin enviar. ¿Desea descartarla y cambiar de modo?"
          : "You have an unsent answer. Discard it and switch modes?",
      );
      if (!confirmed) return;
    }

    if (state.inputMode === "voice") {
      discardVoiceRecording();
    } else {
      dispatch({ type: "TEXT_CHANGED", payload: { value: "" } });
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

  const handleSendVoiceAnswer = useCallback(async () => {
    if (state.phase !== "active") return;
    const transcript = await sendVoiceRecording(code);
    if (!transcript) return;
    requestSubmit(transcript);
  }, [code, requestSubmit, sendVoiceRecording, state.phase]);

  const handleAgreeAndContinue = useCallback(() => {
    if (state.phase !== "active") return;
    requestSubmit(lang === "es" ? "Entiendo" : "I understand");
  }, [lang, requestSubmit, state.phase]);

  const handleReloadPage = useCallback(() => {
    if (typeof window === "undefined") return;
    window.location.reload();
  }, []);

  const timerLabel = formatClock(secondsLeft);
  const textAnswerCharCount = textAnswer.length;
  const isWarning = secondsLeft <= WARNING_AT_SECONDS;
  const isWrapup = secondsLeft <= WRAPUP_AT_SECONDS;
  const isBlinkingTimer = secondsLeft <= 30 && secondsLeft > 0;
  const isSubmittingInQuestionFlow = state.phase === "submitting" && state.submitMode === "question";
  const isAutoSubmitCountdown =
    secondsLeft <= AUTOSUBMIT_SECONDS
    && secondsLeft > 0
    && (state.phase === "active" || state.phase === "submitting");
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
  const voiceCaps = getVoiceCapabilities({
    phase: state.phase,
    submitMode,
    voiceRecorderState,
    voiceBlob,
    voicePreviewUrl,
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

          {state.phase === "submitting" && state.submitMode === "finalAuto" && (
            <InterviewFinalSubmitState
              lang={lang}
              finalSubmitWaitStatus={finalSubmitWaitStatus}
            />
          )}

          {(state.phase === "active" || isSubmittingInQuestionFlow) && currentQuestion && (
            <>
              {isAutoSubmitCountdown && (
                <Alert variant="error" className="mb-4">
                  {lang === "es"
                    ? `Se enviará automáticamente en ${secondsLeft}s...`
                    : `Auto-submitting in ${secondsLeft}s...`}
                </Alert>
              )}

              {isWrapup && !isAutoSubmitCountdown && (
                <Alert variant="error" className="mb-4">
                  {lang === "es"
                    ? "Quedan menos de 5 minutos. Concluya su respuesta actual."
                    : "Less than 5 minutes remaining. Please wrap up your current answer."}
                </Alert>
              )}

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
                      voiceTimerLabel={voiceTimerLabel}
                      canPreviewRecording={voiceCaps.canPreviewRecording}
                      isVoicePreviewPlaying={isVoicePreviewPlaying}
                      onToggleVoicePreviewPlayback={toggleVoicePreviewPlayback}
                      voiceIsRecordingActive={voiceIsRecordingActive}
                      voiceProgressPct={voiceProgressPct}
                      voiceWarningRemaining={voiceWarningRemaining}
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
                      canSendRecording={voiceCaps.canSendRecording}
                      onSendVoiceAnswer={handleSendVoiceAnswer}
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
