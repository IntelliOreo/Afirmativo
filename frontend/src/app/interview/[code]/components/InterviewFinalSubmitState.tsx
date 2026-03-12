"use client";

import { Card } from "@components/Card";
import type { Lang } from "@/lib/language";
import { getInterviewMessages } from "../messages/interviewMessages";

interface InterviewFinalSubmitStateProps {
  lang: Lang;
  finalSubmitWaitStatus: string;
}

export function InterviewFinalSubmitState({
  lang,
  finalSubmitWaitStatus,
}: InterviewFinalSubmitStateProps) {
  const t = getInterviewMessages(lang).finalSubmit;

  return (
    <Card className="text-center py-12">
      <h1 className="text-2xl font-bold text-primary-dark mb-4">
        {t.title}
      </h1>
      <p className="text-primary-darkest mb-6">
        {t.body}
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
