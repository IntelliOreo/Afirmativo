"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import { useParams, useSearchParams } from "next/navigation";
import Link from "next/link";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import { beforeYouStartContent } from "../../../../content/beforeYouStart";
import { DisclaimerConsentPanel } from "./components/DisclaimerConsentPanel";
import { InterviewActiveScreen } from "./components/InterviewActiveScreen";
import { InterviewErrorState } from "./components/InterviewErrorState";
import { InterviewGuardState } from "./components/InterviewGuardState";
import { InterviewProgressHeader } from "./components/InterviewProgressHeader";
import { InterviewQuestionCard } from "./components/InterviewQuestionCard";
import { ReportSection } from "./components/ReportSection";
import { WARNING_AT_SECONDS, WRAPUP_AT_SECONDS } from "./constants";
import { useDisclaimerScrollGate } from "./hooks/useDisclaimerScrollGate";
import { useInterviewLanguage } from "./hooks/useInterviewLanguage";
import { useInterviewMachine } from "./hooks/useInterviewMachine";
import { useInterviewReport } from "./hooks/useInterviewReport";
import { useInterviewWaitingStatus } from "./hooks/useInterviewWaitingStatus";
import {
  getInterviewMessages,
  getReportErrorMessage,
} from "./messages/interviewMessages";
import {
  formatClock,
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

  const isActive = state.phase === "active";
  const isActiveOrSubmitting = isActive || state.phase === "submitting";

  const currentQuestion = isActiveOrSubmitting ? state.question : null;
  const secondsLeft = isActiveOrSubmitting ? state.secondsLeft : 0;
  const answerSecondsLeft = isActiveOrSubmitting ? state.answerSecondsLeft : 0;
  const textAnswer = isActive ? state.textAnswer : "";
  const inputMode = isActive ? state.inputMode : "text";
  const submitMode = state.phase === "submitting" ? state.submitMode : null;
  const completionSource = state.phase === "done" ? state.completionSource : "finished";
  const error = state.phase === "error" ? state.message : "";
  const errorCode = state.phase === "error" ? state.code ?? "" : "";

  const handleTextChange = useCallback((value: string) => {
    dispatch({ type: "TEXT_CHANGED", payload: { value } });
  }, [dispatch]);

  const handleInputModeChange = useCallback((mode: "text" | "voice") => {
    dispatch({ type: "INPUT_MODE_CHANGED", payload: { mode } });
  }, [dispatch]);

  const [hasMicOptIn, setHasMicOptIn] = useState(false);
  const handleMicOptIn = useCallback(() => setHasMicOptIn(true), []);

  const {
    disclaimerScrollRef,
    hasReachedDisclaimerBottom,
    updateDisclaimerScrollState,
  } = useDisclaimerScrollGate(currentQuestion);

  useEffect(() => {
    if (state.phase !== "done") return;
    void resumeReport();
  }, [resumeReport, state.phase]);

  const isTimerExpired = answerSecondsLeft <= 0
    && state.phase === "active"
    && currentQuestion?.kind === "criterion";

  const handleAgreeAndContinue = useCallback(() => {
    if (state.phase !== "active") return;
    requestSubmit(t.page.understandAnswer);
  }, [requestSubmit, state.phase, t.page.understandAnswer]);

  const handleReloadPage = useCallback(() => {
    if (typeof window === "undefined") return;
    window.location.reload();
  }, []);

  const timerLabel = formatClock(secondsLeft);
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
  const reportErrorMessage = getReportErrorMessage(lang, reportError);
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

              {isConsentQuestion ? (
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
                  <InterviewActiveScreen
                    lang={lang}
                    code={code}
                    phase={isSubmittingInQuestionFlow ? "submitting" : "active"}
                    currentQuestion={currentQuestion}
                    textAnswer={textAnswer}
                    inputMode={inputMode}
                    answerSecondsLeft={answerSecondsLeft}
                    isTimerExpired={isTimerExpired}
                    hasMicOptIn={hasMicOptIn}
                    onMicOptIn={handleMicOptIn}
                    onTextChange={handleTextChange}
                    onInputModeChange={handleInputModeChange}
                    requestSubmit={requestSubmit}
                  />

                  {isSubmittingInQuestionFlow && (
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
