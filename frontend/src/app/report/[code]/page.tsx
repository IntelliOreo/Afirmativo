"use client";

import { useState, useEffect } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import { Alert } from "@components/Alert";
import { api } from "@/lib/api";

type ReportStatus = "guard" | "loading" | "generating" | "ready" | "error";

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

export default function ReportPage() {
  const params = useParams();
  const code = params.code as string;

  const [lang, setLang] = useState<"es" | "en">("es");
  const [status, setStatus] = useState<ReportStatus>("guard");
  const [report, setReport] = useState<Report | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    const cookiePin = document.cookie
      .split("; ")
      .find((row) => row.startsWith(`session_${code}=`))
      ?.split("=")[1];

    if (!cookiePin) {
      setStatus("guard");
      return;
    }

    fetchReport();
  }, [code]);

  async function fetchReport() {
    setStatus("loading");
    try {
      const { ok, status: httpStatus, data } = await api<Report & { error?: string }>(`/api/report/${code}`, {
        credentials: "include",
      });

      if (httpStatus === 202) {
        setStatus("generating");
        return;
      }

      if (!ok || !data) {
        throw new Error(data?.error ?? "Failed to load report");
      }

      setReport(data);
      setStatus("ready");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
      setStatus("error");
    }
  }

  function handleDownloadPDF() {
    window.open(`${process.env.NEXT_PUBLIC_API_URL ?? ""}/api/report/${code}/pdf`, "_blank");
  }

  return (
    <div className="flex flex-col min-h-screen">
      <NavHeader lang={lang} onToggleLang={() => setLang(lang === "es" ? "en" : "es")} />

      <main className="flex-1 bg-base-lightest">
        <div className="max-w-2xl mx-auto px-4 py-12">

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
              {lang === "es" ? "Cargando reporte..." : "Loading report..."}
            </p>
          )}

          {status === "generating" && (
            <Card>
              <h1 className="text-2xl font-bold text-primary-dark mb-4">
                {lang === "es" ? "Generando reporte..." : "Generating report..."}
              </h1>
              <p className="text-primary-darkest mb-4">
                {lang === "es"
                  ? "Su reporte de evaluación se está generando. Esto puede tomar unos momentos."
                  : "Your assessment report is being generated. This may take a few moments."}
              </p>
              <Button fullWidth onClick={fetchReport}>
                {lang === "es" ? "Verificar de nuevo" : "Check again"}
              </Button>
            </Card>
          )}

          {status === "error" && (
            <Alert variant="error">
              {lang === "es" ? "Error: " : "Error: "}
              {error}
            </Alert>
          )}

          {status === "ready" && report && (
            <>
              <h1 className="text-3xl font-bold text-primary-dark mb-2">
                {lang === "es" ? "Reporte de Evaluación" : "Assessment Report"}
              </h1>
              <p className="text-sm text-gray-600 mb-6">
                {lang === "es"
                  ? `${report.question_count} preguntas · ${report.duration_minutes} minutos`
                  : `${report.question_count} questions · ${report.duration_minutes} minutes`}
              </p>

              <Card className="mb-6">
                <h2 className="text-xl font-bold text-primary-dark mb-3">
                  {lang === "es" ? "Fortalezas" : "Strengths"}
                </h2>
                <ul className="list-disc list-inside space-y-1 text-primary-darkest">
                  {report.strengths.map((s, i) => (
                    <li key={i}>{s}</li>
                  ))}
                </ul>
              </Card>

              <Card className="mb-6">
                <h2 className="text-xl font-bold text-primary-dark mb-3">
                  {lang === "es" ? "Áreas de mejora" : "Areas for improvement"}
                </h2>
                <ul className="list-disc list-inside space-y-1 text-primary-darkest">
                  {report.weaknesses.map((w, i) => (
                    <li key={i}>{w}</li>
                  ))}
                </ul>
              </Card>

              <Card className="mb-6">
                <h2 className="text-xl font-bold text-primary-dark mb-3">
                  {lang === "es" ? "Recomendación" : "Recommendation"}
                </h2>
                <p className="text-primary-darkest">{report.recommendation}</p>
              </Card>

              <Card className="mb-6">
                <h2 className="text-xl font-bold text-primary-dark mb-3">
                  {lang === "es" ? "Evaluación completa" : "Full assessment"}
                </h2>
                <div className="prose max-w-none text-primary-darkest whitespace-pre-wrap">
                  {lang === "es" ? report.content_es : report.content_en}
                </div>
              </Card>

              <Button fullWidth onClick={handleDownloadPDF}>
                {lang === "es" ? "Descargar reporte en PDF" : "Download report as PDF"}
              </Button>
            </>
          )}

        </div>
      </main>

      <Footer />
    </div>
  );
}
