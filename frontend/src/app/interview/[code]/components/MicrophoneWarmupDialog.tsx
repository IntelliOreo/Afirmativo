"use client";

import { Alert } from "@components/Alert";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import type { Lang } from "@/lib/language";
import {
  getInterviewMessages,
  getMicrophoneWarmupDialogCopy,
} from "../messages/interviewMessages";
import type {
  MicrophoneWarmupDialogMode,
  MicrophoneWarmupDialogState,
} from "../viewTypes";

interface MicrophoneWarmupDialogProps {
  lang: Lang;
  mode: MicrophoneWarmupDialogMode;
  uiState: MicrophoneWarmupDialogState;
  onEnableMicrophone: () => Promise<void>;
  onDismiss: () => void;
}

function AnimatedMicrophoneVisual({ uiState }: { uiState: MicrophoneWarmupDialogState }) {
  const isProcessing = uiState === "warming" || uiState === "recovering";
  const isReady = uiState === "ready_handoff";

  return (
    <div className="mt-5 flex flex-col items-center">
      <div className="relative flex h-28 w-28 items-center justify-center">
        <div
          className={`absolute h-24 w-24 rounded-full transition-all duration-300 ${
            isReady ? "bg-green-100 scale-100" : "bg-primary/10 scale-95"
          } ${isProcessing ? "animate-pulse" : ""}`}
        />
        <div
          className={`absolute h-16 w-16 rounded-full border-2 transition-all duration-300 ${
            isReady ? "border-green-500" : "border-primary/40"
          } ${isProcessing ? "animate-ping" : ""}`}
        />
        {isReady ? (
          <svg viewBox="0 0 48 48" className="relative h-14 w-14 text-green-600">
            <circle cx="24" cy="24" r="20" fill="currentColor" className="opacity-10" />
            <path
              d="M17 24.5l4.5 4.5L31 19.5"
              fill="none"
              stroke="currentColor"
              strokeWidth="4"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        ) : (
          <svg viewBox="0 0 48 48" className={`relative h-14 w-14 text-primary ${isProcessing ? "animate-pulse" : ""}`}>
            <path
              d="M24 31a7 7 0 0 0 7-7V15a7 7 0 1 0-14 0v9a7 7 0 0 0 7 7Z"
              fill="currentColor"
              className="opacity-90"
            />
            <path
              d="M14 22v2a10 10 0 0 0 20 0v-2M24 34v6M18 40h12"
              fill="none"
              stroke="currentColor"
              strokeWidth="3"
              strokeLinecap="round"
            />
          </svg>
        )}
      </div>

      {(uiState === "warming" || uiState === "recovering" || uiState === "ready_handoff") && (
        <div className="mt-4 w-full max-w-xs" aria-live="polite">
          <div className="h-2 overflow-hidden rounded-full bg-base-lighter">
            <div
              role="progressbar"
              aria-valuetext={uiState === "ready_handoff" ? "ready" : "in progress"}
              className={`h-full rounded-full ${
                uiState === "ready_handoff"
                  ? "w-full bg-green-500 transition-all duration-500"
                  : "w-1/2 bg-primary animate-pulse"
              }`}
            />
          </div>
        </div>
      )}
    </div>
  );
}

export function MicrophoneWarmupDialog({
  lang,
  mode,
  uiState,
  onEnableMicrophone,
  onDismiss,
}: MicrophoneWarmupDialogProps) {
  const copy = getMicrophoneWarmupDialogCopy(lang, mode, uiState);
  const t = getInterviewMessages(lang).microphoneDialog;
  const isBusy = uiState === "warming" || uiState === "recovering" || uiState === "ready_handoff";

  return (
    <div
      role="dialog"
      aria-modal="true"
      className="fixed inset-0 z-30 flex items-center justify-center bg-primary-darkest/35 px-4"
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

        <AnimatedMicrophoneVisual uiState={uiState} />

        <Alert variant={copy.statusVariant} className="mt-5">
          {copy.status}
        </Alert>

        <div className="mt-6 flex flex-col gap-3 sm:flex-row sm:justify-end">
          <Button
            type="button"
            variant="secondary"
            onClick={onDismiss}
            disabled={isBusy}
          >
            {mode === "reconnect" ? t.dismissReconnect : t.dismissInitial}
          </Button>
          <Button
            type="button"
            onClick={() => { void onEnableMicrophone(); }}
            disabled={isBusy}
          >
            {copy.buttonLabel}
          </Button>
        </div>
      </Card>
    </div>
  );
}
