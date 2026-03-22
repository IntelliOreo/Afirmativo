"use client";

import { memo } from "react";
import { Card } from "@components/Card";

interface InterviewQuestionCardProps {
  questionText: string;
}

export const InterviewQuestionCard = memo(function InterviewQuestionCard({
  questionText,
}: InterviewQuestionCardProps) {
  return (
    <Card className="mb-6">
      <p className="text-lg font-semibold text-primary-dark whitespace-pre-line">
        {questionText}
      </p>
    </Card>
  );
});
