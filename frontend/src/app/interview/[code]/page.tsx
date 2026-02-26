"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import { Alert } from "@components/Alert";

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "";
const TIMER_TOTAL_SECONDS = 60 * 60; // 60 minutes
const WARNING_AT_SECONDS = 45 * 60;  // warn at 45 min remaining
const WRAPUP_AT_SECONDS = 5 * 60;    // wrap-up at 5 min remaining

type InterviewStatus = "loading" | "active" | "submitting" | "done" | "error";

interface Question {
  id: string;
  textEs: string;
  textEn: string;
  questionNumber: number;
  totalQuestions: number;
}

export default function InterviewPage() {
  const params = useParams();
  const router = useRouter();
  const code = params.code as string;

  const [lang, setLang] = useState<"es" | "en">("es");
  const [status, setStatus] = useState<InterviewStatus>("loading");
  const [question, setQuestion] = useState<Question | null>(null);
  const [textAnswer, setTextAnswer] = useState("");
  const [secondsLeft, setSecondsLeft] = useState(TIMER_TOTAL_SECONDS);
  const [error, setError] = useState("");

  // Timer
  useEffect(() => {
    const interval = setInterval(() => {
      setSecondsLeft((s) => {
        if (s <= 1) {
          clearInterval(interval);
          handleEndInterview();
          return 0;
        }
        return s - 1;
      });
    }, 1000);
    return () => clearInterval(interval);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Start interview on mount
  useEffect(() => {
    async function startInterview() {
      try {
        const res = await fetch(`${API_URL}/api/interview/start`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ sessionCode: code }),
          credentials: "include",
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error ?? "Failed to start");
        setQuestion(data.question);
        setStatus("active");
      } catch (err) {
        setError(err instanceof Error ? err.message : "Unknown error");
        setStatus("error");
      }
    }
    startInterview();
  }, [code]);

  async function handleSubmitAnswer() {
    if (!textAnswer.trim() || status !== "active") return;
    setStatus("submitting");

    try {
      const res = await fetch(`${API_URL}/api/interview/answer`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          sessionCode: code,
          questionId: question?.id,
          answerText: textAnswer.trim(),
        }),
        credentials: "include",
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error ?? "Failed to submit");

      if (data.done) {
        router.push(`/report/${code}`);
      } else {
        setQuestion(data.nextQuestion);
        setTextAnswer("");
        setStatus("active");
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
      setStatus("error");
    }
  }

  async function handleEndInterview() {
    try {
      await fetch(`${API_URL}/api/interview/end`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sessionCode: code }),
        credentials: "include",
      });
    } finally {
      router.push(`/report/${code}`);
    }
  }

  const minutes = Math.floor(secondsLeft / 60);
  const seconds = secondsLeft % 60;
  const timerLabel = `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
  const isWarning = secondsLeft <= WARNING_AT_SECONDS;
  const isWrapup = secondsLeft <= WRAPUP_AT_SECONDS;
  const progressPct = question
    ? (question.questionNumber / question.totalQuestions) * 100
    : 0;

  return (
    <div className="flex flex-col min-h-screen">
      <NavHeader lang={lang} onToggleLang={() => setLang(lang === "es" ? "en" : "es")} />

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

          {(status === "active" || status === "submitting") && question && (
            <>
              {isWrapup && (
                <Alert variant="error" className="mb-4">
                  {lang === "es"
                    ? "Quedan menos de 5 minutos. Concluya su respuesta actual."
                    : "Less than 5 minutes remaining. Please wrap up your current answer."}
                </Alert>
              )}

              <Card className="mb-6">
                <p className="text-lg font-semibold text-primary-dark mb-2">
                  {question.textEs}
                </p>
                <p className="text-base text-gray-600 italic">
                  {question.textEn}
                </p>
              </Card>

              <div className="mb-4">
                <label className="block font-semibold text-primary-darkest mb-2">
                  {lang === "es" ? "Su respuesta" : "Your answer"}
                  <span className="block text-sm font-normal text-gray-500">
                    {lang === "es"
                      ? "Responda en español"
                      : "Please answer in Spanish"}
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
