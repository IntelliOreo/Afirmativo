"use client";

import Link from "next/link";
import { Alert } from "@components/Alert";
import { Button } from "@components/Button";
import type { Lang } from "@/lib/language";
import { getInterviewMessages } from "../messages/interviewMessages";

interface InterviewErrorStateProps {
  lang: Lang;
  code: string;
  error: string;
  isReloadRecoveryError: boolean;
  canRetryPendingAnswer: boolean;
  onRetryPendingAnswer: () => void;
  onReloadPage: () => void;
}

export function InterviewErrorState({
  lang,
  code,
  error,
  isReloadRecoveryError,
  canRetryPendingAnswer,
  onRetryPendingAnswer,
  onReloadPage,
}: InterviewErrorStateProps) {
  const t = getInterviewMessages(lang).error;

  return (
    <>
      <Alert variant="error" className="mb-4">
        {t.prefix}{" "}
        {error}
      </Alert>
      {canRetryPendingAnswer ? (
        <>
          <p className="text-primary-darkest mb-4">
            {t.pendingAnswerBody}
          </p>
          <Button fullWidth className="mb-3" onClick={onRetryPendingAnswer}>
            {t.retryPendingAnswer}
          </Button>
          <Button fullWidth className="mb-3" variant="secondary" onClick={onReloadPage}>
            {t.reloadPage}
          </Button>
        </>
      ) : isReloadRecoveryError ? (
        <>
          <p className="text-primary-darkest mb-4">
            {t.reloadRecoveryBody}
          </p>
          <Button fullWidth className="mb-3" onClick={onReloadPage}>
            {t.reloadPage}
          </Button>
        </>
      ) : (
        <Link href={`/session/${code}`}>
          <Button fullWidth className="mb-3">
            {t.recoverWithPin}
          </Button>
        </Link>
      )}
      <Link href="/">
        <Button fullWidth variant="secondary">
          {t.backHome}
        </Button>
      </Link>
    </>
  );
}
