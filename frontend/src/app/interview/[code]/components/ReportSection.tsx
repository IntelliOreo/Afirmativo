"use client";

import { Alert } from "@components/Alert";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import type { Lang } from "@/lib/language";
import { getInterviewMessages, getReportIntroMessage } from "../messages/interviewMessages";
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
  const t = getInterviewMessages(lang).report;
  const clarityItems = lang === "es" ? report?.areasOfClarityEs ?? [] : report?.areasOfClarity ?? [];
  const developItems =
    lang === "es" ? report?.areasToDevelopFurtherEs ?? [] : report?.areasToDevelopFurther ?? [];
  const recommendation = lang === "es" ? report?.recommendationEs ?? "" : report?.recommendation ?? "";

  return (
    <>
      <Card className="mb-6">
        <h1 className="text-2xl font-bold text-primary-dark mb-4">
          {t.completedTitle}
        </h1>
        <p className="text-primary-darkest mb-6">
          {getReportIntroMessage(lang, completionSource)}
        </p>

        {reportStatus === "idle" && (
          <Button fullWidth onClick={() => { void onLoadReport(); }}>
            {t.generate}
          </Button>
        )}

        {(reportStatus === "loading" || reportStatus === "generating") && (
          <Card className="mb-4 text-center py-10 px-4">
            <p className="text-xs font-semibold uppercase tracking-wider text-primary">
              {reportStatus === "loading"
                ? t.loading
                : t.generating}
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
                  {t.checkAgain}
                </Button>
              </div>
            )}
          </Card>
        )}

        {reportStatus === "error" && (
          <>
            <Alert variant="error" className="mb-4">
              {t.errorPrefix}{" "}
              {reportError}
            </Alert>
            <Button fullWidth onClick={() => { void onLoadReport(); }}>
              {t.tryAgain}
            </Button>
          </>
        )}
      </Card>

      {reportStatus === "ready" && report && (
        <>
          <div className="print-hidden mb-4">
            <Button fullWidth variant="secondary" onClick={onPrintReport}>
              {t.print}
            </Button>
            <p className="text-sm text-gray-600 mt-3">
              {t.printHint}
            </p>
          </div>

          <section className="print-report-area">
            <h2 className="text-2xl sm:text-3xl font-bold text-primary-dark mb-2">
              {t.summaryTitle}
            </h2>
            <p className="text-sm text-gray-600 mb-6">
              {t.questionsMinutes(report.questionCount, report.durationMinutes)}
            </p>

            <Card className="mb-6">
              <h3 className="text-xl font-bold text-primary-dark mb-3">
                {t.clarityTitle}
              </h3>
              {clarityItems.length > 0 ? (
                <ul className="list-disc list-inside space-y-1 text-primary-darkest">
                  {clarityItems.map((strength, index) => (
                    <li key={index}>{strength}</li>
                  ))}
                </ul>
              ) : (
                <p className="text-primary-darkest">
                  {t.noItems}
                </p>
              )}
            </Card>

            <Card className="mb-6">
              <h3 className="text-xl font-bold text-primary-dark mb-3">
                {t.developTitle}
              </h3>
              {developItems.length > 0 ? (
                <ul className="list-disc list-inside space-y-1 text-primary-darkest">
                  {developItems.map((area, index) => (
                    <li key={index}>{area}</li>
                  ))}
                </ul>
              ) : (
                <p className="text-primary-darkest">
                  {t.noItems}
                </p>
              )}
            </Card>

            <Card className="mb-6">
              <h3 className="text-xl font-bold text-primary-dark mb-3">
                {t.recommendationTitle}
              </h3>
              <p className="text-primary-darkest">{recommendation}</p>
            </Card>

            <Card className="mb-6">
              <h3 className="text-xl font-bold text-primary-dark mb-3">
                {t.fullAssessmentTitle}
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
