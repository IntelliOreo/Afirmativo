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

const AUTOSUBMIT_SECONDS = 10; // auto-submit countdown threshold
const WARNING_AT_SECONDS = 45 * 60; // orange bar at 45 min remaining
const WRAPUP_AT_SECONDS = 5 * 60; // red bar + alert at 5 min remaining
const ASYNC_POLL_BACKOFF_MS = [1000, 2000, 3000, 5000, 8000, 15000, 20000, 30000] as const;
const ROTATING_STATUS_MS = 4000;
const ROTATING_STATUS_INITIAL_DELAY_MS = 2000;

const WAITING_STATUS_STRINGS: Record<"es" | "en", string[]> = {
  en: [
    "The interviewer is flipping through your case files... It's a lot of reading... They're doing their best...",
    "Your response is being carefully considered... Good answers have a habit of complicating things — in the best way...",
    "The interviewer reaches for their water... It's their third glass... Hydration is a lifestyle...",
    "Your file is being reviewed, page by page... Whoever put it together really committed to the details...",
    "The interviewer scribbles a note... Their handwriting is truly something... Only they know what it says...",
    "A brief pause to double-check a few things... The interviewer has a reputation for catching the small stuff...",
    "The interviewer leans back, stares at the ceiling for a moment, and thinks... It's part of their process...",
    "Somewhere nearby, a printer is whirring... More documents incoming... There are always more documents...",
    "Your response landed and now the gears are turning... Good things take a moment to process...",
    "The interviewer takes a slow sip of water and exhales... This is their thinking ritual... It usually works...",
    "Case files, sticky notes, and a coffee that went cold an hour ago... The interviewer's desk has seen things...",
    "A quiet moment in the room... The interviewer doesn't mind the silence... You might find it a little awkward...",
    "The interviewer flips back a page... Something caught their eye — probably nothing... Probably...",
    "They've heard a lot of stories in this room... Yours is one of the more interesting ones... That's not nothing...",
    "Almost time for the next question... The interviewer is just finishing a thought — and possibly that glass of water...",
  ],
  es: [
    "El entrevistador está revisando tu expediente... Hay mucho que leer... Está haciendo su mejor esfuerzo...",
    "Tu respuesta está siendo considerada con cuidado... Las buenas respuestas tienen la costumbre de complicar las cosas — de la mejor manera...",
    "El entrevistador toma su vaso de agua... Es el tercero... La hidratación es un estilo de vida...",
    "Tu expediente está siendo revisado, página por página... Quien lo preparó realmente se comprometió con los detalles...",
    "El entrevistador garabatea una nota... Su letra es toda una obra... Solo él sabe lo que dice...",
    "Una breve pausa para verificar algunas cosas... El entrevistador tiene fama de notar los pequeños detalles...",
    "El entrevistador se recuesta, mira el techo un momento y piensa... Es parte de su proceso...",
    "En algún lugar cercano, una impresora zumba... Más documentos en camino... Siempre hay más documentos...",
    "Tu respuesta fue recibida y los engranajes están girando... Las cosas buenas toman un momento para procesarse...",
    "El entrevistador da un lento sorbo de agua y exhala... Es su ritual para pensar... Generalmente funciona...",
    "Expedientes, notas adhesivas y un café que se enfrió hace una hora... El escritorio del entrevistador ha visto cosas...",
    "Un momento de silencio en la sala... Al entrevistador no le molesta el silencio... A ti quizás sí un poco...",
    "El entrevistador regresa una página... Algo llamó su atención — probablemente nada... Probablemente...",
    "Han escuchado muchas historias en esta sala... La tuya es una de las más interesantes... Eso no es poco...",
    "Casi es momento para la siguiente pregunta... El entrevistador está terminando un pensamiento — y posiblemente ese vaso de agua...",
  ],
};

type InterviewStatus = "guard" | "loading" | "active" | "submitting" | "done" | "error";
type ReportStatus = "idle" | "loading" | "generating" | "ready" | "error";
type CompletionSource = "finished" | "already_completed";

interface Question {
  textEs: string;
  textEn: string;
  area: string;
  kind: "disclaimer" | "readiness" | "criterion";
  turnId: string;
  questionNumber: number;
  totalQuestions: number;
}

interface AnswerResponse {
  done: boolean;
  nextQuestion?: Question;
  timerRemainingS: number;
  error?: string;
  code?: string;
}

interface StartResponse {
  question: Question;
  timerRemainingS: number;
  language: "es" | "en";
  error?: string;
  code?: string;
}

type AsyncAnswerJobStatus =
  | "queued"
  | "running"
  | "succeeded"
  | "failed"
  | "conflict"
  | "canceled";

interface AnswerAsyncAcceptedResponse {
  jobId: string;
  clientRequestId: string;
  status: AsyncAnswerJobStatus;
  error?: string;
  code?: string;
}

interface AnswerJobStatusResponse {
  jobId: string;
  clientRequestId: string;
  status: AsyncAnswerJobStatus;
  done: boolean;
  nextQuestion?: Question;
  timerRemainingS: number;
  errorCode?: string;
  errorMessage?: string;
  error?: string;
  code?: string;
}

interface PendingAnswerJob {
  clientRequestId: string;
  turnId: string;
  answerText: string;
  questionText: string;
  jobId?: string;
  createdAt: number;
}

interface Report {
  session_code: string;
  status: string;
  content_en: string;
  content_es: string;
  strengths: string[];
  weaknesses: string[];
  recommendation: string;
  question_count: number;
  duration_minutes: number;
}

type DisclaimerBlock =
  | { type: "paragraph"; text: string }
  | { type: "list"; items: string[] };

function getQuestionTextForLang(
  q: Question | null | undefined,
  language: "es" | "en",
): string {
  if (!q) return "";
  if (language === "es") return q.textEs || q.textEn || "";
  return q.textEn || q.textEs || "";
}

function parseDisclaimerBlocks(rawText: string): DisclaimerBlock[] {
  const lines = rawText.split("\n");
  const blocks: DisclaimerBlock[] = [];
  let currentListItems: string[] = [];

  const flushCurrentList = () => {
    if (currentListItems.length > 0) {
      blocks.push({ type: "list", items: currentListItems });
      currentListItems = [];
    }
  };

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) {
      flushCurrentList();
      continue;
    }

    if (trimmed.startsWith("- ") || trimmed.startsWith("• ")) {
      currentListItems.push(trimmed.replace(/^[-•]\s+/, ""));
      continue;
    }

    flushCurrentList();
    blocks.push({ type: "paragraph", text: trimmed });
  }

  flushCurrentList();
  return blocks;
}

function wait(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function withJitter(ms: number): number {
  const jitterFactor = 0.85 + Math.random() * 0.3;
  return Math.max(250, Math.round(ms * jitterFactor));
}

function makeClientRequestId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function pendingJobStorageKey(sessionCode: string): string {
  return `interview_pending_answer_job_${sessionCode}`;
}

function readPendingAnswerJob(sessionCode: string): PendingAnswerJob | null {
  if (typeof window === "undefined") return null;
  const raw = localStorage.getItem(pendingJobStorageKey(sessionCode));
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw) as PendingAnswerJob;
    if (!parsed?.clientRequestId || !parsed?.turnId) return null;
    return parsed;
  } catch {
    return null;
  }
}

function writePendingAnswerJob(sessionCode: string, pending: PendingAnswerJob): void {
  if (typeof window === "undefined") return;
  localStorage.setItem(pendingJobStorageKey(sessionCode), JSON.stringify(pending));
}

function clearPendingAnswerJob(sessionCode: string): void {
  if (typeof window === "undefined") return;
  localStorage.removeItem(pendingJobStorageKey(sessionCode));
}

function randomMessageIndex(currentIndex: number, total: number): number {
  if (total <= 1) return 0;
  let nextIndex = Math.floor(Math.random() * total);
  while (nextIndex === currentIndex) {
    nextIndex = Math.floor(Math.random() * total);
  }
  return nextIndex;
}

function useRotatingStatus(
  messages: string[],
  active: boolean,
  intervalMs = ROTATING_STATUS_MS,
  initialDelayMs = ROTATING_STATUS_INITIAL_DELAY_MS,
): string {
  const [index, setIndex] = useState(0);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    if (!active || messages.length === 0) {
      setIndex(0);
      setVisible(false);
      return;
    }

    setIndex(0);
    setVisible(false);

    let interval: number | undefined;
    const delay = window.setTimeout(() => {
      setIndex(() => randomMessageIndex(-1, messages.length));
      setVisible(true);
      interval = window.setInterval(() => {
        setIndex((currentIndex) => randomMessageIndex(currentIndex, messages.length));
      }, intervalMs);
    }, initialDelayMs);

    return () => {
      window.clearTimeout(delay);
      if (interval !== undefined) {
        window.clearInterval(interval);
      }
    };
  }, [active, initialDelayMs, intervalMs, messages]);

  return visible ? (messages[index] ?? "") : "";
}

function InterviewPageContent() {
  const params = useParams();
  const searchParams = useSearchParams();
  const code = params.code as string;
  const requestedLang = searchParams.get("lang");

  const [lang, setLang] = useState<"es" | "en">("es");
  const [langInitialized, setLangInitialized] = useState(false);
  const [status, setStatus] = useState<InterviewStatus>("guard");
  const [question, setQuestion] = useState<Question | null>(null);
  const [textAnswer, setTextAnswer] = useState("");
  const [secondsLeft, setSecondsLeft] = useState(0);
  const [error, setError] = useState("");
  const [completionSource, setCompletionSource] = useState<CompletionSource>("finished");
  const [reportStatus, setReportStatus] = useState<ReportStatus>("idle");
  const [report, setReport] = useState<Report | null>(null);
  const [reportError, setReportError] = useState("");
  const [forceSubmit, setForceSubmit] = useState(false);
  const [hasReachedDisclaimerBottom, setHasReachedDisclaimerBottom] = useState(false);
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

  const markInterviewDone = useCallback((source: CompletionSource) => {
    setCompletionSource(source);
    setStatus("done");
    setQuestion(null);
    setTextAnswer("");
    setSecondsLeft(0);
    setForceSubmit(false);
    setError("");
    resetReportState();
  }, [resetReportState]);

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
      const { ok, status: httpStatus, data } = await api<Report & { error?: string }>(`/api/report/${code}`, {
        credentials: "include",
      });

      if (httpStatus === 202) {
        setReportStatus("generating");
        return;
      }

      if (!ok || !data) {
        throw new Error(data?.error ?? "Failed to load report");
      }

      setReport(data);
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
    let attempt = 0;
    while (true) {
      if (isCanceled()) {
        throw new Error("Polling canceled");
      }

      const { ok, status: httpStatus, data } = await api<AnswerJobStatusResponse>(
        `/api/interview/answer-jobs/${encodeURIComponent(jobId)}?sessionCode=${encodeURIComponent(code)}`,
        { credentials: "include" },
      );
      if (!ok || !data) {
        throw new Error(data?.error ?? (httpStatus === 404 ? "Async answer job not found" : "Failed to poll answer status"));
      }

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
      if (data.status === "conflict") {
        throw new Error(data.errorMessage || "Turn is stale or out of order");
      }
      throw new Error(data.errorMessage || data.errorCode || "Failed to process answer");
    }
  }, [code]);

  const submitPendingAnswerJob = useCallback(async (
    pending: PendingAnswerJob,
    isCanceled: () => boolean = () => false,
  ): Promise<AnswerResponse> => {
    let current = pending;
    if (!current.jobId) {
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
      if (!ok || !data) throw new Error(data?.error ?? "Failed to queue answer");

      current = {
        ...current,
        clientRequestId: data.clientRequestId,
        jobId: data.jobId,
      };
      writePendingAnswerJob(code, current);
    }

    return pollAsyncAnswerJob(current.jobId, isCanceled);
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
      setError(err instanceof Error ? err.message : "Unknown error");
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

  // Session guard: check for session cookie before starting.
  useEffect(() => {
    if (!langInitialized) return;

    const cookiePin = document.cookie
      .split("; ")
      .find((row) => row.startsWith(`session_${code}=`))
      ?.split("=")[1];

    if (!cookiePin) {
      setStatus("guard");
      return;
    }

    async function startInterview() {
      setStatus("loading");
      setError("");

      try {
        const { ok, status: httpStatus, data } = await api<StartResponse>("/api/interview/start", {
          method: "POST",
          body: { sessionCode: code, language: langRef.current },
          credentials: "include",
        });

        if (!ok || !data) {
          const errorMessage = data?.error ?? "Failed to start";
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
            setError(err instanceof Error ? err.message : "Unknown error");
            setStatus("error");
          } finally {
            restoringPendingRef.current = false;
          }
          return;
        }

        setStatus("active");
      } catch (err) {
        setError(err instanceof Error ? err.message : "Unknown error");
        setStatus("error");
      }
    }

    void startInterview();
  }, [applyAnswerOutcome, code, langInitialized, markInterviewDone, resetReportState, submitPendingAnswerJob]);

  async function handleSubmitAnswer() {
    if (!textAnswer.trim()) return;
    await submitAnswer(textAnswer);
  }

  async function handleAgreeAndContinue() {
    const agreementText = lang === "es" ? "Entiendo" : "I understand";
    await submitAnswer(agreementText);
  }

  const minutes = Math.floor(secondsLeft / 60);
  const seconds = secondsLeft % 60;
  const timerLabel = `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
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
  const waitStrings = WAITING_STATUS_STRINGS[lang];
  const startupWaitStatus = useRotatingStatus(waitStrings, status === "loading");
  const questionWaitStatus = useRotatingStatus(waitStrings, isSubmittingInQuestionFlow);
  const finalSubmitWaitStatus = useRotatingStatus(waitStrings, status === "submitting" && forceSubmit);
  const reportWaitStatus = useRotatingStatus(
    waitStrings,
    reportStatus === "loading" || reportStatus === "generating",
  );

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
              <Link href={`/session/${code}`}>
                <Button fullWidth className="mb-3">
                  {lang === "es" ? "Recuperar sesión con PIN" : "Recover session with PIN"}
                </Button>
              </Link>
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

                {reportStatus === "loading" && (
                  <>
                    <p className="text-primary-darkest mb-2">
                      {lang === "es" ? "Cargando reporte..." : "Loading report..."}
                    </p>
                    {reportWaitStatus && (
                      <p className="text-base sm:text-lg text-primary-dark leading-snug">
                        {reportWaitStatus}
                      </p>
                    )}
                  </>
                )}

                {reportStatus === "generating" && (
                  <>
                    <p className="text-primary-darkest mb-4">
                      {lang === "es"
                        ? "Su reporte se está generando. Esto puede tomar unos momentos."
                        : "Your report is being generated. This may take a few moments."}
                    </p>
                    {reportWaitStatus && (
                      <p className="text-base sm:text-lg text-primary-dark leading-snug mb-4">
                        {reportWaitStatus}
                      </p>
                    )}
                    <Button fullWidth onClick={loadReport}>
                      {lang === "es" ? "Verificar de nuevo" : "Check again"}
                    </Button>
                  </>
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
                      {lang === "es" ? "Reporte de Evaluación" : "Assessment Report"}
                    </h2>
                    <p className="text-sm text-gray-600 mb-6">
                      {lang === "es"
                        ? `${report.question_count} preguntas · ${report.duration_minutes} minutos`
                        : `${report.question_count} questions · ${report.duration_minutes} minutes`}
                    </p>

                    <Card className="mb-6">
                      <h3 className="text-xl font-bold text-primary-dark mb-3">
                        {lang === "es" ? "Fortalezas" : "Strengths"}
                      </h3>
                      {report.strengths.length > 0 ? (
                        <ul className="list-disc list-inside space-y-1 text-primary-darkest">
                          {report.strengths.map((s, i) => (
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
                        {lang === "es" ? "Áreas de mejora" : "Areas for improvement"}
                      </h3>
                      {report.weaknesses.length > 0 ? (
                        <ul className="list-disc list-inside space-y-1 text-primary-darkest">
                          {report.weaknesses.map((w, i) => (
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
                      onChange={(e) => setTextAnswer(e.target.value)}
                      rows={6}
                      className="w-full px-3 py-3 text-base border border-base-lighter rounded focus:outline-none focus:ring-2 focus:ring-primary resize-none"
                      placeholder={
                        lang === "es"
                          ? "Escriba su respuesta aquí..."
                          : "Type your answer here..."
                      }
                    />
                  </div>

                  <Button
                    fullWidth
                    disabled={!textAnswer.trim()}
                    onClick={handleSubmitAnswer}
                  >
                    {lang === "es" ? "Enviar respuesta" : "Submit answer"}
                  </Button>
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
