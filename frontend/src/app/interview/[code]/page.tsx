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
}

interface StartResponse {
  question: Question;
  timerRemainingS: number;
  language: "es" | "en";
  error?: string;
  code?: string;
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

  // Auto-submit at timer expiry: process the final answer and move to done state.
  const autoSubmit = useCallback(async () => {
    if (statusRef.current === "submitting" || statusRef.current === "done") return;
    setForceSubmit(true);
    setStatus("submitting");

    try {
      const { ok, data } = await api<AnswerResponse>("/api/interview/answer", {
        method: "POST",
        body: {
          sessionCode: code,
          answerText: textAnswerRef.current.trim(),
          questionText: getQuestionTextForLang(questionRef.current, langRef.current),
          turnId: questionRef.current?.turnId ?? "",
        },
        credentials: "include",
      });
      if (!ok || !data) throw new Error(data?.error ?? "Failed to submit");
      markInterviewDone("finished");
    } catch {
      // If final submit fails, backend usually already ended the flow due timeout.
      markInterviewDone("finished");
    }
  }, [code, markInterviewDone]);

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

        setStatus("active");
      } catch (err) {
        setError(err instanceof Error ? err.message : "Unknown error");
        setStatus("error");
      }
    }

    void startInterview();
  }, [code, langInitialized, markInterviewDone, resetReportState]);

  async function submitAnswer(answerValue: string) {
    if (status !== "active") return;
    setStatus("submitting");

    try {
      const { ok, data } = await api<AnswerResponse>("/api/interview/answer", {
        method: "POST",
        body: {
          sessionCode: code,
          answerText: answerValue.trim(),
          questionText: getQuestionTextForLang(question, lang),
          turnId: question?.turnId ?? "",
        },
        credentials: "include",
      });

      if (!ok || !data) throw new Error(data?.error ?? "Failed to submit");

      if (data.done) {
        markInterviewDone("finished");
      } else {
        setQuestion(data.nextQuestion ?? null);
        setTextAnswer("");
        setSecondsLeft(data.timerRemainingS);
        setStatus("active");
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
      setStatus("error");
    }
  }

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

  return (
    <div className="flex flex-col min-h-screen">
      <NavHeader lang={lang} />

      {showInterviewProgress && (
        <>
          {/* Timer bar */}
          <div
            className={`flex items-center justify-between px-4 py-2 text-sm font-semibold ${
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
        </>
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
            <p className="text-primary-darkest">
              {lang === "es" ? "Cargando..." : "Loading..."}
            </p>
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
                  <p className="text-primary-darkest">
                    {lang === "es" ? "Cargando reporte..." : "Loading report..."}
                  </p>
                )}

                {reportStatus === "generating" && (
                  <>
                    <p className="text-primary-darkest mb-4">
                      {lang === "es"
                        ? "Su reporte se está generando. Esto puede tomar unos momentos."
                        : "Your report is being generated. This may take a few moments."}
                    </p>
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
                  <h2 className="text-3xl font-bold text-primary-dark mb-2">
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

              {isConsentQuestion ? (
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
                    disabled={status === "submitting" || !hasReachedDisclaimerBottom}
                    onClick={handleAgreeAndContinue}
                  >
                    {status === "submitting"
                      ? lang === "es"
                        ? "Enviando..."
                        : "Submitting..."
                      : lang === "es"
                        ? "Entiendo"
                        : "I understand"}
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
                      disabled={status === "submitting"}
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
                    disabled={status === "submitting" || !textAnswer.trim()}
                    onClick={handleSubmitAnswer}
                  >
                    {status === "submitting"
                      ? lang === "es"
                        ? "Enviando..."
                        : "Submitting..."
                      : lang === "es"
                        ? "Enviar respuesta"
                        : "Submit answer"}
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
