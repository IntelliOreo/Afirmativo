"use client";

import type { Lang } from "@/lib/language";
import { getInterviewMessages } from "../messages/interviewMessages";
import type { Question } from "../models";

interface InterviewProgressHeaderProps {
  lang: Lang;
  isBlinkingTimer: boolean;
  isWrapup: boolean;
  isWarning: boolean;
  timerLabel: string;
  question: Question | null;
  progressPct: number;
}

function getTimerBannerTone(isWrapup: boolean, isWarning: boolean): string {
  if (isWrapup) return "bg-error text-white";
  if (isWarning) return "bg-accent-warm text-white";
  return "bg-primary-dark text-white";
}

export function InterviewProgressHeader({
  lang,
  isBlinkingTimer,
  isWrapup,
  isWarning,
  timerLabel,
  question: _question,
  progressPct,
}: InterviewProgressHeaderProps) {
  const t = getInterviewMessages(lang).progress;

  return (
    <div className={isBlinkingTimer ? "animate-pulse" : ""}>
      <div
        className={`flex flex-col items-start gap-1 px-4 py-2 text-sm font-semibold sm:flex-row sm:items-center sm:justify-between ${getTimerBannerTone(isWrapup, isWarning)}`}
      >
        <span>
          {t.timeRemaining}: {timerLabel}
        </span>
      </div>

      <div className="h-1 bg-base-lighter">
        <div
          className="h-1 bg-primary transition-all duration-500"
          style={{ width: `${progressPct}%` }}
        />
      </div>
    </div>
  );
}
