"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import { Alert } from "@components/Alert";
import { api } from "@/lib/api";

const AUTOSUBMIT_SECONDS = 10;   // auto-submit countdown threshold
const WARNING_AT_SECONDS = 45 * 60;  // orange bar at 45 min remaining
const WRAPUP_AT_SECONDS = 5 * 60;    // red bar + alert at 5 min remaining

type InterviewStatus = "guard" | "loading" | "active" | "submitting" | "done" | "error";

interface Question {
  textEs: string;
  textEn: string;
  area: string;
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
}

function getQuestionTextForLang(
  q: Question | null | undefined,
  language: "es" | "en",
): string {
  if (!q) return "";
  if (language === "es") return q.textEs || q.textEn || "";
  return q.textEn || q.textEs || "";
}

export default function InterviewPage() {
  const params = useParams();
  const searchParams = useSearchParams();
  const router = useRouter();
  const code = params.code as string;
  const requestedLang = searchParams.get("lang");

  const [lang, setLang] = useState<"es" | "en">(() => {
    if (requestedLang === "en" || requestedLang === "es") return requestedLang;
    if (typeof window !== "undefined") {
      const stored = sessionStorage.getItem(`interview_lang_${code}`);
      if (stored === "en" || stored === "es") return stored;
    }
    return "es";
  });
  const [status, setStatus] = useState<InterviewStatus>("guard");
  const [question, setQuestion] = useState<Question | null>(null);
  const [textAnswer, setTextAnswer] = useState("");
  const [secondsLeft, setSecondsLeft] = useState(0);
  const [error, setError] = useState("");
  const [forceSubmit, setForceSubmit] = useState(false);

  // Ref to access latest textAnswer and question inside timer callback
  const textAnswerRef = useRef(textAnswer);
  textAnswerRef.current = textAnswer;
  const questionRef = useRef(question);
  questionRef.current = question;
  const langRef = useRef(lang);
  langRef.current = lang;
  const statusRef = useRef(status);
  statusRef.current = status;

  useEffect(() => {
    if (typeof window !== "undefined") {
      sessionStorage.setItem(`interview_lang_${code}`, lang);
    }
  }, [code, lang]);

  // Auto-submit at timer expiry: processes the final answer through AI, then redirects.
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
          questionNumber: questionRef.current?.questionNumber ?? 0,
          questionText: getQuestionTextForLang(questionRef.current, langRef.current),
        },
        credentials: "include",
      });
      if (!ok || !data) throw new Error(data?.error ?? "Failed to submit");
      router.push(`/report/${code}`);
    } catch {
      // If the call fails, redirect to report anyway — session is done server-side.
      router.push(`/report/${code}`);
    }
  }, [code, router]);

  // Countdown timer — ticks every second, auto-submits at 0
  useEffect(() => {
    if (status === "done" || status === "loading" || status === "guard") return;
    if (secondsLeft <= 0 && status === "active") {
      autoSubmit();
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

  // Session guard: check for session cookie before starting
  useEffect(() => {
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
      try {
        const { ok, data } = await api<StartResponse>("/api/interview/start", {
          method: "POST",
          body: { sessionCode: code, language: langRef.current },
          credentials: "include",
        });
        if (!ok || !data) throw new Error(data?.error ?? "Failed to start");
        setQuestion(data.question);
        setSecondsLeft(data.timerRemainingS);
        if (data.language === "en" || data.language === "es") {
          setLang(data.language);
        }
        setStatus("active");
      } catch (err) {
        setError(err instanceof Error ? err.message : "Unknown error");
        setStatus("error");
      }
    }
    startInterview();
  }, [code]);

  // Manual answer submission
  async function handleSubmitAnswer() {
    if (!textAnswer.trim() || status !== "active") return;
    setStatus("submitting");

    try {
      const { ok, data } = await api<AnswerResponse>("/api/interview/answer", {
        method: "POST",
        body: {
          sessionCode: code,
          answerText: textAnswer.trim(),
          questionNumber: question?.questionNumber ?? 0,
          questionText: getQuestionTextForLang(question, lang),
        },
        credentials: "include",
      });
      if (!ok || !data) throw new Error(data?.error ?? "Failed to submit");

      if (data.done) {
        router.push(`/report/${code}`);
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

  const minutes = Math.floor(secondsLeft / 60);
  const seconds = secondsLeft % 60;
  const timerLabel = `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
  const isWarning = secondsLeft <= WARNING_AT_SECONDS;
  const isWrapup = secondsLeft <= WRAPUP_AT_SECONDS;
  const isAutoSubmitCountdown = secondsLeft <= AUTOSUBMIT_SECONDS && secondsLeft > 0 && (status === "active" || status === "submitting");
  const progressPct = question
    ? (question.questionNumber / question.totalQuestions) * 100
    : 0;

  return (
    <div className="flex flex-col min-h-screen">
      <NavHeader lang={lang} />

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
            {lang === "es" ? "Pregunta" : "Question"} {question.questionNumber}{" "}
            / {question.totalQuestions}
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
            <Alert variant="error">
              {lang === "es" ? "Error: " : "Error: "}
              {error}
            </Alert>
          )}

          {status === "done" && (
            <Card>
              <h1 className="text-2xl font-bold text-primary-dark mb-4">
                {lang === "es" ? "Entrevista finalizada" : "Interview ended"}
              </h1>
              <p className="text-primary-darkest mb-4">
                {lang === "es"
                  ? "Su entrevista de práctica ha terminado."
                  : "Your practice interview has ended."}
              </p>
              <Alert variant="info" className="mb-6">
                {lang === "es"
                  ? "La generación del reporte aún no está disponible. Esta función está en desarrollo."
                  : "Report generation is not yet available. This feature is under development."}
              </Alert>
              <Link href="/">
                <Button fullWidth>
                  {lang === "es" ? "Volver al inicio" : "Back to home"}
                </Button>
              </Link>
            </Card>
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
              {/* Auto-submit countdown banner */}
              {isAutoSubmitCountdown && (
                <Alert variant="error" className="mb-4">
                  {lang === "es"
                    ? `Se enviará automáticamente en ${secondsLeft}s...`
                    : `Auto-submitting in ${secondsLeft}s...`}
                </Alert>
              )}

              {/* Wrap-up warning (5 min) — only if not already showing auto-submit countdown */}
              {isWrapup && !isAutoSubmitCountdown && (
                <Alert variant="error" className="mb-4">
                  {lang === "es"
                    ? "Quedan menos de 5 minutos. Concluya su respuesta actual."
                    : "Less than 5 minutes remaining. Please wrap up your current answer."}
                </Alert>
              )}

              <Card className="mb-6">
                <p className="text-lg font-semibold text-primary-dark">
                  {lang === "es" ? question.textEs : question.textEn}
                </p>
              </Card>

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
        </div>
      </main>

      <Footer />
    </div>
  );
}
