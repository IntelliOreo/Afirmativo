"use client";

import { Card } from "@components/Card";
import type { Lang } from "@/lib/language";

interface InterviewFinalSubmitStateProps {
  lang: Lang;
  finalSubmitWaitStatus: string;
}

export function InterviewFinalSubmitState({
  lang,
  finalSubmitWaitStatus,
}: InterviewFinalSubmitStateProps) {
  return (
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
  );
}
