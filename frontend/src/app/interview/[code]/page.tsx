"use client";

import { Suspense, useState, useEffect, useRef, useCallback } from "react";
import { useParams, useSearchParams } from "next/navigation";
import Link from "next/link";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import { Alert } from "@components/Alert";
import { api } from "@/lib/api";
import { parseLang, writeStoredLang } from "@/lib/language";
import { beforeYouStartContent } from "../../../../content/beforeYouStart";
import {
  AUTOSUBMIT_SECONDS,
  ASYNC_POLL_BACKOFF_MS,
  ASYNC_POLL_CIRCUIT_BREAKER_COOLDOWN_MS,
  ASYNC_POLL_CIRCUIT_BREAKER_FAILURES,
  ASYNC_POLL_TIMEOUT_MS,
  TEXT_ANSWER_MAX_CHARS,
  VOICE_MAX_SECONDS,
  VOICE_WAVE_BARS,
  WARNING_AT_SECONDS,
  WRAPUP_AT_SECONDS,
} from "./constants";
import { useInterviewWaitingStatus } from "./hooks/useInterviewWaitingStatus";
import { useVoiceRecorder } from "./hooks/useVoiceRecorder";
import type {
  AnswerAsyncAcceptedResponse,
  AnswerJobStatusResponse,
  AnswerResponse,
  CompletionSource,
  InputMode,
  InterviewStatus,
  Lang,
  PendingAnswerJob,
  Question,
  Report,
  ReportStatus,
  StartResponse,
} from "./types";
import {
  buildCodedError,
  clearPendingAnswerJob,
  extractErrorCode,
  formatBytes,
  formatClock,
  getQuestionTextForLang,
  isReloadRecoveryErrorCode,
  makeClientRequestId,
  parseDisclaimerBlocks,
  readPendingAnswerJob,
  shouldClearPendingAnswerOnError,
  wait,
  withJitter,
  writePendingAnswerJob,
} from "./utils";

type ReportAPIResponse = Report & {
  strengths?: string[];
  weaknesses?: string[];
};

function InterviewPageContent() {
  const params = useParams();
  const searchParams = useSearchParams();
  const code = params.code as string;
  const requestedLang = searchParams.get("lang");

  const [lang, setLang] = useState<Lang>("es");
  const [langInitialized, setLangInitialized] = useState(false);
  const [status, setStatus] = useState<InterviewStatus>("guard");
  const [question, setQuestion] = useState<Question | null>(null);
  const [textAnswer, setTextAnswer] = useState("");
  const [secondsLeft, setSecondsLeft] = useState(0);
  const [error, setError] = useState("");
  const [errorCode, setErrorCode] = useState("");
  const [completionSource, setCompletionSource] = useState<CompletionSource>("finished");
  const [reportStatus, setReportStatus] = useState<ReportStatus>("idle");
  const [report, setReport] = useState<Report | null>(null);
  const [reportError, setReportError] = useState("");
  const [forceSubmit, setForceSubmit] = useState(false);
  const [hasReachedDisclaimerBottom, setHasReachedDisclaimerBottom] = useState(false);
  const [inputMode, setInputMode] = useState<InputMode>("text");
  const disclaimerScrollRef = useRef<HTMLDivElement | null>(null);

  // Refs to access latest state values inside async callbacks.
  const textAnswerRef = useRef(textAnswer);
  textAnswerRef.current = textAnswer;
  const questionRef = useRef(question);
  questionRef.current = question;
  const langRef = useRef(lang);
  langRef.current = lang;
  const statusRef = useRef(status);
  statusRef.current = status;
  const restoringPendingRef = useRef(false);

  const resetReportState = useCallback(() => {
    setReportStatus("idle");
    setReport(null);
    setReportError("");
  }, []);

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
    isActive: status === "active",
  });

  const handleInputModeSwitch = useCallback((nextMode: InputMode) => {
    if (nextMode === inputMode) return;
    if (
      voiceRecorderState === "recording"
      || voiceRecorderState === "paused"
      || voiceRecorderState === "sending"
    ) {
      setVoiceErrorMessage(
        langRef.current === "es"
          ? "Detenga la grabación antes de cambiar de modo."
          : "Stop recording before switching modes.",
      );
      return;
    }

    const hasUnsentText = inputMode === "text" && textAnswerRef.current.trim().length > 0;
    const hasUnsentVoice = inputMode === "voice" && !!voiceBlob;
    if ((hasUnsentText || hasUnsentVoice) && typeof window !== "undefined") {
      const confirmed = window.confirm(
        langRef.current === "es"
          ? "Tiene una respuesta sin enviar. ¿Desea descartarla y cambiar de modo?"
          : "You have an unsent answer. Discard it and switch modes?",
      );
      if (!confirmed) return;
    }

    if (inputMode === "voice") {
      discardVoiceRecording();
    } else {
      setTextAnswer("");
    }
    setInputMode(nextMode);
  }, [discardVoiceRecording, inputMode, setVoiceErrorMessage, voiceBlob, voiceRecorderState]);

  const markInterviewDone = useCallback((source: CompletionSource) => {
    discardVoiceRecording();
    setInputMode("text");
    setCompletionSource(source);
    setStatus("done");
    setQuestion(null);
    setTextAnswer("");
    setSecondsLeft(0);
    setForceSubmit(false);
    setError("");
    setErrorCode("");
    resetReportState();
  }, [discardVoiceRecording, resetReportState]);

  useEffect(() => {
    const langFromQuery = parseLang(requestedLang);
    if (langFromQuery) {
      setLang(langFromQuery);
      setLangInitialized(true);
      return;
    }

    if (typeof window !== "undefined") {
      const storedInterviewLang = parseLang(sessionStorage.getItem(`interview_lang_${code}`));
      if (storedInterviewLang) {
        setLang(storedInterviewLang);
        setLangInitialized(true);
        return;
      }
      const storedUiLang = parseLang(sessionStorage.getItem("ui_lang"));
      if (storedUiLang) {
        setLang(storedUiLang);
      }
    }

    setLangInitialized(true);
  }, [code, requestedLang]);

  useEffect(() => {
    if (!langInitialized) return;
    if (typeof window !== "undefined") {
      writeStoredLang(lang);
      sessionStorage.setItem(`interview_lang_${code}`, lang);
    }
  }, [code, lang, langInitialized]);

  useEffect(() => {
    if (question?.kind === "disclaimer") {
      setHasReachedDisclaimerBottom(false);
    }
  }, [question?.kind]);

  useEffect(() => {
    setInputMode("text");
    discardVoiceRecording();
  }, [discardVoiceRecording, question?.turnId]);

  const updateDisclaimerScrollState = useCallback(() => {
    const el = disclaimerScrollRef.current;
    if (!el) return;
    const noScrollNeeded = el.scrollHeight <= el.clientHeight + 4;
    const atBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 4;
    if (noScrollNeeded || atBottom) {
      setHasReachedDisclaimerBottom(true);
    }
  }, []);

  useEffect(() => {
    if (question?.kind !== "disclaimer") return;
    const id = window.requestAnimationFrame(updateDisclaimerScrollState);
    return () => window.cancelAnimationFrame(id);
  }, [question?.kind, question?.textEn, question?.textEs, updateDisclaimerScrollState]);

  const loadReport = useCallback(async () => {
    setReportError("");
    setReportStatus("loading");

    try {
      const { ok, status: httpStatus, data } = await api<ReportAPIResponse & { error?: string }>(`/api/report/${code}`, {
        credentials: "include",
      });

      if (httpStatus === 202) {
        setReportStatus("generating");
        return;
      }

      if (!ok || !data) {
        throw new Error(data?.error ?? "Failed to load report");
      }

      const normalizedReport: Report = {
        ...data,
        areas_of_clarity: data.areas_of_clarity ?? data.strengths ?? [],
        areas_to_develop_further: data.areas_to_develop_further ?? data.weaknesses ?? [],
      };
      setReport(normalizedReport);
      setReportStatus("ready");
    } catch (err) {
      setReportError(err instanceof Error ? err.message : "Unknown error");
      setReportStatus("error");
    }
  }, [code]);

  const handlePrintReport = useCallback(() => {
    if (typeof window === "undefined") return;
    window.print();
  }, []);

  const applyAnswerOutcome = useCallback((data: AnswerResponse) => {
    if (data.done) {
      markInterviewDone("finished");
      return;
    }
    setQuestion(data.nextQuestion ?? null);
    setTextAnswer("");
    setSecondsLeft(data.timerRemainingS);
    setStatus("active");
  }, [markInterviewDone]);

  const pollAsyncAnswerJob = useCallback(async (
    jobId: string,
    isCanceled: () => boolean = () => false,
  ): Promise<AnswerResponse> => {
    const startedAt = Date.now();
    let attempt = 0;
    let consecutiveTransientFailures = 0;

    while (true) {
      if (isCanceled()) {
        throw new Error("Polling canceled");
      }

      if (Date.now() - startedAt >= ASYNC_POLL_TIMEOUT_MS) {
        throw buildCodedError("Polling timed out before the answer job completed", "ASYNC_POLL_TIMEOUT");
      }

      try {
        const { ok, status: httpStatus, data } = await api<AnswerJobStatusResponse>(
          `/api/interview/answer-jobs/${encodeURIComponent(jobId)}?sessionCode=${encodeURIComponent(code)}`,
          { credentials: "include" },
        );

        if (!ok || !data) {
          const isTransientStatus = httpStatus === 429 || httpStatus >= 500;
          if (isTransientStatus) {
            consecutiveTransientFailures += 1;
            if (consecutiveTransientFailures >= ASYNC_POLL_CIRCUIT_BREAKER_FAILURES) {
              await wait(withJitter(ASYNC_POLL_CIRCUIT_BREAKER_COOLDOWN_MS));
              throw buildCodedError(
                "Polling paused after repeated server failures. Reload to retry.",
                "ASYNC_POLL_CIRCUIT_OPEN",
              );
            }

            const transientDelay = ASYNC_POLL_BACKOFF_MS[Math.min(attempt, ASYNC_POLL_BACKOFF_MS.length - 1)];
            attempt += 1;
            await wait(withJitter(transientDelay));
            continue;
          }

          throw buildCodedError(
            data?.error ?? (httpStatus === 404 ? "Async answer job not found" : "Failed to poll answer status"),
            data?.code,
          );
        }

        consecutiveTransientFailures = 0;

        if (data.status === "queued" || data.status === "running") {
          const delay = ASYNC_POLL_BACKOFF_MS[Math.min(attempt, ASYNC_POLL_BACKOFF_MS.length - 1)];
          attempt += 1;
          await wait(withJitter(delay));
          continue;
        }

        if (data.status === "succeeded") {
          clearPendingAnswerJob(code);
          return {
            done: data.done,
            nextQuestion: data.nextQuestion,
            timerRemainingS: data.timerRemainingS,
          };
        }

        clearPendingAnswerJob(code);
        if (data.status === "canceled") {
          throw buildCodedError(
            data.errorMessage || "Processing was canceled. Reload to continue.",
            data.errorCode || "AI_RETRY_EXHAUSTED",
          );
        }
        if (data.status === "conflict") {
          throw buildCodedError(data.errorMessage || "Turn is stale or out of order", data.errorCode || "TURN_CONFLICT");
        }
        throw buildCodedError(data.errorMessage || data.errorCode || "Failed to process answer", data.errorCode);
      } catch (err) {
        if (err instanceof TypeError) {
          consecutiveTransientFailures += 1;
          if (consecutiveTransientFailures >= ASYNC_POLL_CIRCUIT_BREAKER_FAILURES) {
            await wait(withJitter(ASYNC_POLL_CIRCUIT_BREAKER_COOLDOWN_MS));
            throw buildCodedError(
              "Polling paused after repeated network failures. Reload to retry.",
              "ASYNC_POLL_CIRCUIT_OPEN",
            );
          }

          const transientDelay = ASYNC_POLL_BACKOFF_MS[Math.min(attempt, ASYNC_POLL_BACKOFF_MS.length - 1)];
          attempt += 1;
          await wait(withJitter(transientDelay));
          continue;
        }

        throw err;
      }
    }
  }, [code]);

  const submitPendingAnswerJob = useCallback(async (
    pending: PendingAnswerJob,
    isCanceled: () => boolean = () => false,
  ): Promise<AnswerResponse> => {
    let current = pending;
    let jobId = current.jobId;
    if (!jobId) {
      const { ok, data } = await api<AnswerAsyncAcceptedResponse>("/api/interview/answer-async", {
        method: "POST",
        body: {
          sessionCode: code,
          answerText: current.answerText,
          questionText: current.questionText,
          turnId: current.turnId,
          clientRequestId: current.clientRequestId,
        },
        credentials: "include",
      });
      if (!ok || !data) throw buildCodedError(data?.error ?? "Failed to queue answer", data?.code);

      current = {
        ...current,
        clientRequestId: data.clientRequestId,
        jobId: data.jobId,
      };
      jobId = data.jobId;
      writePendingAnswerJob(code, current);
    }

    return pollAsyncAnswerJob(jobId, isCanceled);
  }, [code, pollAsyncAnswerJob]);

  const submitAnswer = useCallback(async (
    answerValue: string,
    opts?: { fallbackDoneOnError?: boolean },
  ) => {
    if (statusRef.current !== "active") return;

    const answerText = answerValue.trim();
    const turnId = questionRef.current?.turnId ?? "";
    const questionText = getQuestionTextForLang(questionRef.current, langRef.current);
    const pending: PendingAnswerJob = {
      clientRequestId: makeClientRequestId(),
      turnId,
      answerText,
      questionText,
      createdAt: Date.now(),
    };

    setStatus("submitting");
    setError("");
    setErrorCode("");
    writePendingAnswerJob(code, pending);

    try {
      const result = await submitPendingAnswerJob(pending);
      applyAnswerOutcome(result);
    } catch (err) {
      if (opts?.fallbackDoneOnError) {
        clearPendingAnswerJob(code);
        markInterviewDone("finished");
        return;
      }
      const errCode = extractErrorCode(err);
      if (shouldClearPendingAnswerOnError(errCode)) {
        clearPendingAnswerJob(code);
      }
      setError(err instanceof Error ? err.message : "Unknown error");
      setErrorCode(errCode);
      setStatus("error");
    }
  }, [applyAnswerOutcome, code, markInterviewDone, submitPendingAnswerJob]);

  // Auto-submit at timer expiry: process final answer and move to done state on fallback.
  const autoSubmit = useCallback(async () => {
    if (statusRef.current === "submitting" || statusRef.current === "done") return;
    setForceSubmit(true);
    await submitAnswer(textAnswerRef.current.trim(), { fallbackDoneOnError: true });
  }, [submitAnswer]);

  // Countdown timer — ticks every second, auto-submits at 0.
  useEffect(() => {
    if (status === "done" || status === "loading" || status === "guard" || status === "error") {
      return;
    }

    if (secondsLeft <= 0 && status === "active") {
      void autoSubmit();
      return;
    }

    const interval = setInterval(() => {
      setSecondsLeft((s) => {
        if (s <= 1) {
          clearInterval(interval);
          return 0;
        }
        return s - 1;
      });
    }, 1000);

    return () => clearInterval(interval);
  }, [status, secondsLeft <= 0, autoSubmit]); // eslint-disable-line react-hooks/exhaustive-deps

  // Session guard: start interview only if backend auth cookie is valid.
  useEffect(() => {
    if (!langInitialized) return;

    async function startInterview() {
      setStatus("loading");
      setError("");
      setErrorCode("");

      try {
        const { ok, status: httpStatus, data } = await api<StartResponse>("/api/interview/start", {
          method: "POST",
          body: { sessionCode: code, language: langRef.current },
          credentials: "include",
        });

        if (!ok || !data) {
          const errorMessage = data?.error ?? "Failed to start";
          const unauthorized =
            httpStatus === 401
            || data?.code === "UNAUTHORIZED"
            || data?.code === "SESSION_MISMATCH";
          if (unauthorized) {
            setStatus("guard");
            return;
          }
          const completed =
            httpStatus === 409
            || data?.code === "INTERVIEW_COMPLETED"
            || errorMessage.toLowerCase().includes("completed");

          if (completed) {
            markInterviewDone("already_completed");
            return;
          }

          throw new Error(errorMessage);
        }

        setCompletionSource("finished");
        resetReportState();
        setQuestion(data.question);
        setTextAnswer("");
        setSecondsLeft(data.timerRemainingS);
        setForceSubmit(false);

        if (data.language === "en" || data.language === "es") {
          setLang(data.language);
        }

        const pending = readPendingAnswerJob(code);
        if (pending && !restoringPendingRef.current) {
          restoringPendingRef.current = true;
          setStatus("submitting");
          try {
            const resumedResult = await submitPendingAnswerJob(pending);
            applyAnswerOutcome(resumedResult);
          } catch (err) {
            const errCode = extractErrorCode(err);
            if (shouldClearPendingAnswerOnError(errCode)) {
              clearPendingAnswerJob(code);
            }
            setError(err instanceof Error ? err.message : "Unknown error");
            setErrorCode(errCode);
            setStatus("error");
          } finally {
            restoringPendingRef.current = false;
          }
          return;
        }

        setStatus("active");
      } catch (err) {
        setError(err instanceof Error ? err.message : "Unknown error");
        setErrorCode(extractErrorCode(err));
        setStatus("error");
      }
    }

    void startInterview();
  }, [applyAnswerOutcome, code, langInitialized, markInterviewDone, resetReportState, submitPendingAnswerJob]);

  async function handleSubmitAnswer() {
    if (!textAnswer.trim()) return;
    await submitAnswer(textAnswer);
  }

  async function handleSendVoiceAnswer() {
    const transcript = await sendVoiceRecording(code);
    if (!transcript) return;
    await submitAnswer(transcript);
  }

  async function handleAgreeAndContinue() {
    const agreementText = lang === "es" ? "Entiendo" : "I understand";
    await submitAnswer(agreementText);
  }

  const timerLabel = formatClock(secondsLeft);
  const textAnswerCharCount = textAnswer.length;
  const isWarning = secondsLeft <= WARNING_AT_SECONDS;
  const isWrapup = secondsLeft <= WRAPUP_AT_SECONDS;
  const isBlinkingTimer = secondsLeft <= 30 && secondsLeft > 0;
  const isAutoSubmitCountdown =
    secondsLeft <= AUTOSUBMIT_SECONDS
    && secondsLeft > 0
    && (status === "active" || status === "submitting");
  const isConsentQuestion = question?.kind === "disclaimer";
  const consentQuestionText = getQuestionTextForLang(question, lang);
  const consentBlocks = parseDisclaimerBlocks(consentQuestionText);
  const progressPct = question
    ? (question.questionNumber / question.totalQuestions) * 100
    : 0;
  const showInterviewProgress = status === "active" || (status === "submitting" && !forceSubmit);
  const isSubmittingInQuestionFlow = status === "submitting" && !forceSubmit;
  const isReloadRecoveryError = isReloadRecoveryErrorCode(errorCode);
  const isVoiceMode = inputMode === "voice" && !isConsentQuestion;
  const voiceTimerLabel = formatClock(voiceDurationSeconds);
  const voiceProgressPct = Math.min(100, (voiceDurationSeconds / VOICE_MAX_SECONDS) * 100);
  const voiceWarningRemaining = voiceWarningSeconds == null
    ? null
    : Math.max(0, VOICE_MAX_SECONDS - voiceWarningSeconds);
  const canSwitchModes =
    !isSubmittingInQuestionFlow
    && voiceRecorderState !== "recording"
    && voiceRecorderState !== "paused"
    && voiceRecorderState !== "sending";
  const canToggleRecording =
    status === "active"
    && (
      voiceRecorderState === "idle"
      || voiceRecorderState === "recording"
      || voiceRecorderState === "paused"
    );
  const canCompleteRecording =
    status === "active"
    && (voiceRecorderState === "recording" || voiceRecorderState === "paused");
  const canDiscardRecording =
    status === "active"
    && (
      voiceRecorderState === "recording"
      || voiceRecorderState === "paused"
      || voiceRecorderState === "stopped"
    );
  const canSendRecording =
    status === "active"
    && voiceRecorderState === "stopped"
    && !!voiceBlob
    && voiceBlob.size > 0;
  const canPreviewRecording =
    status === "active"
    && (voiceRecorderState === "paused" || voiceRecorderState === "stopped")
    && !!voicePreviewUrl;
  const centerControlLabel = voiceRecorderState === "idle"
    ? "Record"
    : voiceRecorderState === "recording"
      ? "Pause"
      : "Resume";
  const {
    startupWaitStatus,
    questionWaitStatus,
    finalSubmitWaitStatus,
    reportWaitStatus,
  } = useInterviewWaitingStatus({
    lang,
    interviewStatus: status,
    isSubmittingInQuestionFlow,
    forceSubmit,
    reportStatus,
  });

  return (
    <div className="interview-page flex flex-col min-h-screen">
      <NavHeader lang={lang} />

      {showInterviewProgress && (
        <div className={isBlinkingTimer ? "animate-pulse" : ""}>
          {/* Timer bar */}
          <div
            className={`flex flex-col items-start gap-1 px-4 py-2 text-sm font-semibold sm:flex-row sm:items-center sm:justify-between ${
              isWrapup
                ? "bg-error text-white"
                : isWarning
                  ? "bg-accent-warm text-white"
                  : "bg-primary-dark text-white"
            }`}
          >
            <span>
              {lang === "es" ? "Tiempo restante" : "Time remaining"}: {timerLabel}
            </span>
            {question && (
              <span>
                {lang === "es" ? "Pregunta" : "Question"} {question.questionNumber} / {question.totalQuestions}
              </span>
            )}
          </div>

          {/* Progress bar */}
          <div className="h-1 bg-base-lighter">
            <div
              className="h-1 bg-primary transition-all duration-500"
              style={{ width: `${progressPct}%` }}
            />
          </div>
        </div>
      )}

      <main className="flex-1 bg-base-lightest">
        <div className="max-w-2xl mx-auto px-4 py-8">
          {status === "guard" && (
            <Card>
              <h1 className="text-2xl font-bold text-primary-dark mb-4">
                {lang === "es" ? "Sesión no encontrada" : "Session not found"}
              </h1>
              <p className="text-primary-darkest mb-6">
                {lang === "es"
                  ? "No se encontró una sesión activa en este navegador. Si tiene un código de sesión y PIN, puede recuperar su acceso."
                  : "No active session found in this browser. If you have a session code and PIN, you can recover your access."}
              </p>
              <Link href={`/session/${code}`}>
                <Button fullWidth>
                  {lang === "es" ? "Recuperar sesión" : "Recover session"}
                </Button>
              </Link>
            </Card>
          )}

          {status === "loading" && (
            <Card className="text-center py-8">
              <p className="text-primary-darkest mb-3">
                {lang === "es" ? "Cargando..." : "Loading..."}
              </p>
              {startupWaitStatus && (
                <p className="text-base sm:text-lg text-primary-dark leading-snug">{startupWaitStatus}</p>
              )}
            </Card>
          )}

          {status === "error" && (
            <>
              <Alert variant="error" className="mb-4">
                {lang === "es" ? "Error: " : "Error: "}
                {error}
              </Alert>
              {isReloadRecoveryError ? (
                <>
                  <p className="text-primary-darkest mb-4">
                    {lang === "es"
                      ? "Esta sesión se desincronizó. Recargue esta página para obtener el estado más reciente de la entrevista."
                      : "This session got out of sync. Reload this page to fetch the latest interview state."}
                  </p>
                  <Button
                    fullWidth
                    className="mb-3"
                    onClick={() => {
                      if (typeof window !== "undefined") window.location.reload();
                    }}
                  >
                    {lang === "es" ? "Recargar página" : "Reload page"}
                  </Button>
                </>
              ) : (
                <Link href={`/session/${code}`}>
                  <Button fullWidth className="mb-3">
                    {lang === "es" ? "Recuperar sesión con PIN" : "Recover session with PIN"}
                  </Button>
                </Link>
              )}
              <Link href="/">
                <Button fullWidth variant="secondary">
                  {lang === "es" ? "Volver al inicio" : "Back to home"}
                </Button>
              </Link>
            </>
          )}

          {status === "done" && (
            <>
              <Card className="mb-6">
                <h1 className="text-2xl font-bold text-primary-dark mb-4">
                  {lang === "es" ? "Entrevista completada" : "Interview completed"}
                </h1>
                <p className="text-primary-darkest mb-6">
                  {completionSource === "already_completed"
                    ? lang === "es"
                      ? "Esta entrevista ya estaba finalizada. Puede generar su reporte aquí mismo."
                      : "This interview was already completed. You can generate your report here."
                    : lang === "es"
                      ? "Todos los criterios fueron evaluados. Cuando esté listo, genere su reporte."
                      : "All criteria were evaluated. Generate your report when you are ready."}
                </p>

                {reportStatus === "idle" && (
                  <Button fullWidth onClick={loadReport}>
                    {lang === "es" ? "Generar reporte" : "Generate report"}
                  </Button>
                )}

                {(reportStatus === "loading" || reportStatus === "generating") && (
                  <Card className="mb-4 text-center py-10 px-4">
                    <p className="text-xs font-semibold uppercase tracking-wider text-primary">
                      {reportStatus === "loading"
                        ? (lang === "es" ? "Cargando reporte" : "Loading report")
                        : (lang === "es" ? "Generando reporte" : "Generating report")}
                    </p>
                    {reportWaitStatus && (
                      <p className="mt-3 text-base sm:text-lg text-primary-dark leading-snug">
                        {reportWaitStatus}
                      </p>
                    )}
                    <div className="mt-6 inline-block h-8 w-8 border-4 border-primary border-t-transparent rounded-full animate-spin" />
                    {reportStatus === "generating" && (
                      <div className="mt-6">
                        <Button fullWidth onClick={loadReport}>
                          {lang === "es" ? "Verificar de nuevo" : "Check again"}
                        </Button>
                      </div>
                    )}
                  </Card>
                )}

                {reportStatus === "error" && (
                  <>
                    <Alert variant="error" className="mb-4">
                      {lang === "es" ? "Error: " : "Error: "}
                      {reportError}
                    </Alert>
                    <Button fullWidth onClick={loadReport}>
                      {lang === "es" ? "Intentar de nuevo" : "Try again"}
                    </Button>
                  </>
                )}
              </Card>

              {reportStatus === "ready" && report && (
                <>
                  <div className="print-hidden mb-4">
                    <Button fullWidth variant="secondary" onClick={handlePrintReport}>
                      {lang === "es" ? "Imprimir / Guardar como PDF" : "Print / Save as PDF"}
                    </Button>
                    <p className="text-sm text-gray-600 mt-3">
                      {lang === "es"
                        ? "En móvil: toque Imprimir y luego Guardar como PDF."
                        : "On mobile: tap Print, then Save as PDF."}
                    </p>
                  </div>

                  <section className="print-report-area">
                    <h2 className="text-2xl sm:text-3xl font-bold text-primary-dark mb-2">
                      {lang === "es" ? "Resumen de retroalimentación para preparación" : "Preparation feedback summary"}
                    </h2>
                    <p className="text-sm text-gray-600 mb-6">
                      {lang === "es"
                        ? `${report.question_count} preguntas · ${report.duration_minutes} minutos`
                        : `${report.question_count} questions · ${report.duration_minutes} minutes`}
                    </p>

                    <Card className="mb-6">
                      <h3 className="text-xl font-bold text-primary-dark mb-3">
                        {lang === "es" ? "Áreas de claridad" : "Areas of clarity"}
                      </h3>
                      {report.areas_of_clarity.length > 0 ? (
                        <ul className="list-disc list-inside space-y-1 text-primary-darkest">
                          {report.areas_of_clarity.map((s, i) => (
                            <li key={i}>{s}</li>
                          ))}
                        </ul>
                      ) : (
                        <p className="text-primary-darkest">
                          {lang === "es" ? "Sin elementos para mostrar." : "No items to display."}
                        </p>
                      )}
                    </Card>

                    <Card className="mb-6">
                      <h3 className="text-xl font-bold text-primary-dark mb-3">
                        {lang === "es" ? "Áreas para desarrollar más" : "Areas to develop further"}
                      </h3>
                      {report.areas_to_develop_further.length > 0 ? (
                        <ul className="list-disc list-inside space-y-1 text-primary-darkest">
                          {report.areas_to_develop_further.map((w, i) => (
                            <li key={i}>{w}</li>
                          ))}
                        </ul>
                      ) : (
                        <p className="text-primary-darkest">
                          {lang === "es" ? "Sin elementos para mostrar." : "No items to display."}
                        </p>
                      )}
                    </Card>

                    <Card className="mb-6">
                      <h3 className="text-xl font-bold text-primary-dark mb-3">
                        {lang === "es" ? "Recomendación" : "Recommendation"}
                      </h3>
                      <p className="text-primary-darkest">{report.recommendation}</p>
                    </Card>

                    <Card className="mb-6">
                      <h3 className="text-xl font-bold text-primary-dark mb-3">
                        {lang === "es" ? "Evaluación completa" : "Full assessment"}
                      </h3>
                      <div className="prose max-w-none text-primary-darkest whitespace-pre-wrap">
                        {lang === "es" ? report.content_es : report.content_en}
                      </div>
                    </Card>
                  </section>
                </>
              )}

              <Link href="/">
                <Button fullWidth variant="secondary">
                  {lang === "es" ? "Volver al inicio" : "Back to home"}
                </Button>
              </Link>
            </>
          )}

          {status === "submitting" && forceSubmit && (
            <Card className="text-center py-12">
              <h1 className="text-2xl font-bold text-primary-dark mb-4">
                {lang === "es" ? "Tiempo agotado" : "Time is up"}
              </h1>
              <p className="text-primary-darkest mb-6">
                {lang === "es"
                  ? "Enviando su última respuesta para evaluación..."
                  : "Submitting your final answer for evaluation..."}
              </p>
              {finalSubmitWaitStatus && (
                <p className="text-base sm:text-lg text-primary-dark leading-snug mb-6">
                  {finalSubmitWaitStatus}
                </p>
              )}
              <div className="inline-block h-8 w-8 border-4 border-primary border-t-transparent rounded-full animate-spin" />
            </Card>
          )}

          {(status === "active" || (status === "submitting" && !forceSubmit)) && question && (
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

              <Card className="mb-6">
                {isConsentQuestion ? (
                  <div
                    ref={disclaimerScrollRef}
                    onScroll={updateDisclaimerScrollState}
                    className="max-h-72 overflow-y-auto pr-1"
                  >
                    <div className="space-y-4 text-base font-normal text-primary-darkest leading-relaxed">
                      {consentBlocks.map((block, index) =>
                        block.type === "paragraph" ? (
                          <p key={`p-${index}`} className="whitespace-pre-line">
                            {block.text}
                          </p>
                        ) : (
                          <ul key={`l-${index}`} className="list-disc list-inside space-y-2">
                            {block.items.map((item, itemIndex) => (
                              <li key={`li-${index}-${itemIndex}`}>{item}</li>
                            ))}
                          </ul>
                        ),
                      )}
                    </div>
                  </div>
                ) : (
                  <p className="text-lg font-semibold text-primary-dark whitespace-pre-line">
                    {lang === "es" ? question.textEs : question.textEn}
                  </p>
                )}
              </Card>

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
                <>
                  <Alert variant="warning" className="mb-6">
                    {beforeYouStartContent[lang].warningAlert}
                  </Alert>

                  {!hasReachedDisclaimerBottom && (
                    <p className="text-sm text-primary-darkest mb-4">
                      {lang === "es"
                        ? "Desplácese hasta el final del aviso para continuar."
                        : "Scroll to the bottom of the disclaimer to continue."}
                    </p>
                  )}

                  <Button
                    fullWidth
                    disabled={!hasReachedDisclaimerBottom}
                    onClick={handleAgreeAndContinue}
                  >
                    {lang === "es" ? "Entiendo" : "I understand"}
                  </Button>
                </>
              ) : (
                <>
                  <div className="mb-6 grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <Button
                      type="button"
                      variant={inputMode === "text" ? "primary" : "secondary"}
                      disabled={!canSwitchModes}
                      onClick={() => handleInputModeSwitch("text")}
                    >
                      {lang === "es" ? "Entrada por texto" : "Text input"}
                    </Button>
                    <Button
                      type="button"
                      variant={inputMode === "voice" ? "primary" : "secondary"}
                      disabled={!canSwitchModes}
                      onClick={() => handleInputModeSwitch("voice")}
                    >
                      {lang === "es" ? "Entrada por voz" : "Voice input"}
                    </Button>
                  </div>

                  {!isVoiceMode ? (
                    <>
                      <div className="mb-4">
                        <label className="block font-semibold text-primary-darkest mb-2">
                          {lang === "es" ? "Su respuesta" : "Your answer"}
                          <span className="block text-sm font-normal text-gray-500">
                            {lang === "es"
                              ? "Responda en su idioma seleccionado"
                              : "Please answer in your selected language"}
                          </span>
                        </label>
                        <textarea
                          value={textAnswer}
                          onChange={(e) => setTextAnswer(e.target.value.slice(0, TEXT_ANSWER_MAX_CHARS))}
                          maxLength={TEXT_ANSWER_MAX_CHARS}
                          rows={6}
                          className="w-full px-3 py-3 text-base border border-base-lighter rounded focus:outline-none focus:ring-2 focus:ring-primary resize-none"
                          placeholder={
                            lang === "es"
                              ? "Escriba su respuesta aquí..."
                              : "Type your answer here..."
                          }
                        />
                        <p className="mt-2 text-right text-sm text-primary-darkest">
                          {lang === "es" ? "Caracteres" : "Characters"}: {textAnswerCharCount} / {TEXT_ANSWER_MAX_CHARS}
                        </p>
                      </div>

                      <Button
                        fullWidth
                        disabled={!textAnswer.trim()}
                        onClick={handleSubmitAnswer}
                      >
                        {lang === "es" ? "Enviar respuesta" : "Submit answer"}
                      </Button>
                    </>
                  ) : (
                    <>
                      <Card className="mb-4">
                        <div className="mb-4 flex items-center justify-center gap-3">
                          <p className="text-center text-5xl font-bold tracking-wide text-primary">
                            {voiceTimerLabel}
                          </p>
                          <button
                            type="button"
                            className="h-9 w-9 rounded-full border border-primary text-primary text-sm font-bold disabled:opacity-40 disabled:cursor-not-allowed"
                            aria-label={lang === "es" ? "Reproducir audio grabado" : "Play recorded audio"}
                            disabled={!canPreviewRecording}
                            onClick={() => void toggleVoicePreviewPlayback()}
                          >
                            {isVoicePreviewPlaying ? "II" : ">"}
                          </button>
                        </div>
                        <div className="mb-5 flex items-end justify-center gap-1 h-8">
                          {VOICE_WAVE_BARS.map((bar, idx) => (
                            <span
                              // Decorative waveform bars for recorder dashboard.
                              key={`voice-wave-${idx}`}
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
                          {voiceRecorderState === "idle"
                            ? (lang === "es" ? "Pulse Record para empezar." : "Press Record to begin.")
                            : voiceIsRecordingPaused
                              ? (lang === "es" ? "Grabación en pausa." : "Recording paused.")
                              : voiceIsRecordingActive
                                ? (lang === "es" ? "Grabando..." : "Recording...")
                                : (lang === "es" ? "Grabación completa. Envíe cuando esté listo." : "Recording complete. Send when ready.")}
                        </p>

                        {voiceBlob && (
                          <p className="text-sm text-primary-darkest mb-4">
                            {lang === "es" ? "Audio listo" : "Audio ready"}: {formatBytes(voiceBlob.size)}
                          </p>
                        )}

                        <div className="mb-5 flex items-start justify-center gap-6 sm:gap-10">
                          <div className="flex flex-col items-center gap-2">
                            <Button
                              type="button"
                              variant="secondary"
                              className="!h-14 !w-14 !rounded-full !px-0 !py-0 shadow-sm"
                              disabled={!canDiscardRecording}
                              onClick={discardVoiceRecording}
                            >
                              ×
                            </Button>
                            <span className="text-xs font-semibold text-primary-darkest">
                              {lang === "es" ? "Discard" : "Discard"}
                            </span>
                          </div>

                          <div className="flex flex-col items-center gap-2">
                            <Button
                              type="button"
                              variant="danger"
                              className="!h-16 !w-16 !rounded-full !px-0 !py-0 shadow-md"
                              disabled={!canToggleRecording}
                              onClick={() => void startVoiceRecording()}
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
                              onClick={completeVoiceRecording}
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
                          disabled={!canSendRecording}
                          onClick={() => void handleSendVoiceAnswer()}
                        >
                          {voiceRecorderState === "sending"
                            ? (lang === "es" ? "Enviando..." : "Sending...")
                            : (lang === "es" ? "Send recording" : "Send recording")}
                        </Button>
                      </Card>
                    </>
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
