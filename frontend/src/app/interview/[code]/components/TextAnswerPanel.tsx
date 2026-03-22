"use client";

import { Button } from "@components/Button";
import { Card } from "@components/Card";
import type { Lang } from "@/lib/language";
import { getInterviewMessages } from "../messages/interviewMessages";

interface TextAnswerPanelProps {
  lang: Lang;
  answerTimerLabel: string;
  answerTimerTone: "normal" | "warning" | "danger";
  answerTimerMessage: string;
  textAnswer: string;
  textAnswerCharCount: number;
  maxChars: number;
  isReadOnly?: boolean;
  isTimerExpired?: boolean;
  onTextAnswerChange: (nextValue: string) => void;
  onSubmitAnswer: () => void | Promise<void>;
}

export function TextAnswerPanel({
  lang,
  answerTimerLabel,
  answerTimerTone,
  answerTimerMessage,
  textAnswer,
  textAnswerCharCount,
  maxChars,
  isReadOnly = false,
  isTimerExpired = false,
  onTextAnswerChange,
  onSubmitAnswer,
}: TextAnswerPanelProps) {
  const t = getInterviewMessages(lang).textAnswer;
  const timerToneClass =
    answerTimerTone === "danger"
      ? "border-danger bg-danger-lightest text-danger-dark"
      : answerTimerTone === "warning"
        ? "border-yellow-300 bg-yellow-50 text-yellow-900"
        : "border-primary/20 bg-primary/5 text-primary-darkest";

  return (
    <Card className="mb-4">
      <div className={`mb-4 rounded-lg border px-4 py-3 ${timerToneClass}`}>
        <p className="text-xs font-semibold uppercase tracking-wide">
          {t.submitWindowLabel}
        </p>
        <p className="mt-1 text-2xl font-bold">{answerTimerLabel}</p>
        <p className="mt-2 text-sm leading-snug">{answerTimerMessage}</p>
      </div>

      <div className="mb-4">
        <label className="block font-semibold text-primary-darkest mb-2">
          {t.finalDraft}
          <span className="block text-sm font-normal text-gray-500">
            {t.answerInSelectedLanguage}
          </span>
        </label>
        <textarea
          value={textAnswer}
          onChange={(e) => onTextAnswerChange(e.target.value.slice(0, maxChars))}
          maxLength={maxChars}
          rows={6}
          readOnly={isReadOnly || isTimerExpired}
          className="w-full px-3 py-3 text-base border border-base-lighter rounded focus:outline-none focus:ring-2 focus:ring-primary resize-none"
          placeholder={t.placeholder}
        />
        <p className="mt-2 text-right text-sm text-primary-darkest">
          {t.characters}: {textAnswerCharCount} / {maxChars}
        </p>
      </div>

      <Button fullWidth disabled={!textAnswer.trim() || isReadOnly} onClick={() => { void onSubmitAnswer(); }}>
        {t.submit}
      </Button>
    </Card>
  );
}
