"use client";

import { Alert } from "@components/Alert";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import type { Lang } from "@/lib/language";
import { getTimeoutDialogCopy } from "../messages/interviewMessages";

interface TimeoutDialogProps {
  lang: Lang;
  isInterrupted: boolean;
  isTranscribing: boolean;
  onSubmit: () => void;
}

function TimeoutDialogVisual({ isBusy }: { isBusy: boolean }) {
  return (
    <div className="mt-5 flex flex-col items-center">
      <div className="relative flex h-28 w-28 items-center justify-center">
        <div
          className={`absolute h-24 w-24 rounded-full transition-all duration-300 ${
            isBusy ? "bg-primary/10 scale-95 animate-pulse" : "bg-danger-lightest scale-100"
          }`}
        />
        <div
          className={`absolute h-16 w-16 rounded-full border-2 transition-all duration-300 ${
            isBusy ? "border-primary/40 animate-ping" : "border-danger/40"
          }`}
        />
        {isBusy ? (
          <div className="relative h-12 w-12 rounded-full border-4 border-primary border-t-transparent animate-spin" />
        ) : (
          <svg viewBox="0 0 48 48" className="relative h-14 w-14 text-danger-dark">
            <circle cx="24" cy="24" r="16" fill="none" stroke="currentColor" strokeWidth="3" opacity="0.25" />
            <path
              d="M24 14v11l7 4"
              fill="none"
              stroke="currentColor"
              strokeWidth="4"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        )}
      </div>

      <div className="mt-4 w-full max-w-xs" aria-live="polite">
        <div className="h-2 overflow-hidden rounded-full bg-base-lighter">
          <div
            role="progressbar"
            className={`h-full rounded-full ${
              isBusy ? "w-1/2 bg-primary animate-pulse" : "w-full bg-danger"
            }`}
          />
        </div>
      </div>
    </div>
  );
}

export function TimeoutDialog({
  lang,
  isInterrupted,
  isTranscribing,
  onSubmit,
}: TimeoutDialogProps) {
  const copy = getTimeoutDialogCopy(lang, { isInterrupted, isTranscribing });

  return (
    <div
      role="dialog"
      aria-modal="true"
      className="fixed inset-0 z-40 flex items-center justify-center bg-primary-darkest/35 px-4"
    >
      <Card className="w-full max-w-lg shadow-lg">
        <p className="text-xs font-semibold uppercase tracking-wide text-primary">
          {copy.eyebrow}
        </p>
        <h2 className="mt-2 text-2xl font-bold text-primary-darkest">
          {copy.title}
        </h2>
        <p className="mt-3 text-base leading-relaxed text-primary-darkest">
          {copy.body}
        </p>

        <TimeoutDialogVisual isBusy={isTranscribing} />

        <Alert variant={copy.statusVariant} className="mt-5">
          {copy.status}
        </Alert>

        <div className="mt-6 flex justify-end">
          <Button
            type="button"
            onClick={onSubmit}
            disabled={isTranscribing}
          >
            {copy.buttonLabel}
          </Button>
        </div>
      </Card>
    </div>
  );
}
