"use client";

import { Alert } from "@components/Alert";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import type { Lang } from "@/lib/language";
import type { InterviewReport } from "../models";
import type { CompletionSource, ReportStatus } from "../viewTypes";

interface ReportSectionProps {
  completionSource: CompletionSource;
  report: InterviewReport | null;
  reportError: string;
  reportStatus: ReportStatus;
  reportWaitStatus: string;
  lang: Lang;
  onLoadReport: () => void | Promise<void>;
  onCheckAgain: () => void | Promise<void>;
  onPrintReport: () => void;
}

export function ReportSection({
  completionSource,
  report,
  reportError,
  reportStatus,
  reportWaitStatus,
  lang,
  onLoadReport,
  onCheckAgain,
  onPrintReport,
}: ReportSectionProps) {
  const clarityItems = lang === "es" ? report?.areasOfClarityEs ?? [] : report?.areasOfClarity ?? [];
  const developItems =
    lang === "es" ? report?.areasToDevelopFurtherEs ?? [] : report?.areasToDevelopFurther ?? [];
  const recommendation = lang === "es" ? report?.recommendationEs ?? "" : report?.recommendation ?? "";

  return (
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
          <Button fullWidth onClick={() => { void onLoadReport(); }}>
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
                <Button fullWidth onClick={() => { void onCheckAgain(); }}>
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
            <Button fullWidth onClick={() => { void onLoadReport(); }}>
              {lang === "es" ? "Intentar de nuevo" : "Try again"}
            </Button>
          </>
        )}
      </Card>

      {reportStatus === "ready" && report && (
        <>
          <div className="print-hidden mb-4">
            <Button fullWidth variant="secondary" onClick={onPrintReport}>
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
                ? `${report.questionCount} preguntas · ${report.durationMinutes} minutos`
                : `${report.questionCount} questions · ${report.durationMinutes} minutes`}
            </p>

            <Card className="mb-6">
              <h3 className="text-xl font-bold text-primary-dark mb-3">
                {lang === "es" ? "Áreas de claridad" : "Areas of clarity"}
              </h3>
              {clarityItems.length > 0 ? (
                <ul className="list-disc list-inside space-y-1 text-primary-darkest">
                  {clarityItems.map((strength, index) => (
                    <li key={index}>{strength}</li>
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
              {developItems.length > 0 ? (
                <ul className="list-disc list-inside space-y-1 text-primary-darkest">
                  {developItems.map((area, index) => (
                    <li key={index}>{area}</li>
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
              <p className="text-primary-darkest">{recommendation}</p>
            </Card>

            <Card className="mb-6">
              <h3 className="text-xl font-bold text-primary-dark mb-3">
                {lang === "es" ? "Evaluación completa" : "Full assessment"}
              </h3>
              <div className="prose max-w-none text-primary-darkest whitespace-pre-wrap">
                {lang === "es" ? report.contentEs : report.contentEn}
              </div>
            </Card>
          </section>
        </>
      )}
    </>
  );
}
