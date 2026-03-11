"use client";

import Link from "next/link";
import { Alert } from "@components/Alert";
import { Button } from "@components/Button";
import type { Lang } from "@/lib/language";

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
  return (
    <>
      <Alert variant="error" className="mb-4">
        {lang === "es" ? "Error: " : "Error: "}
        {error}
      </Alert>
      {canRetryPendingAnswer ? (
        <>
          <p className="text-primary-darkest mb-4">
            {lang === "es"
              ? "Tiene una respuesta pendiente guardada. Intente reenviarla o recargue la página para continuar."
              : "A pending answer is still saved. Retry sending it or reload the page to continue."}
          </p>
          <Button fullWidth className="mb-3" onClick={onRetryPendingAnswer}>
            {lang === "es" ? "Reintentar envío" : "Retry send"}
          </Button>
          <Button fullWidth className="mb-3" variant="secondary" onClick={onReloadPage}>
            {lang === "es" ? "Recargar página" : "Reload page"}
          </Button>
        </>
      ) : isReloadRecoveryError ? (
        <>
          <p className="text-primary-darkest mb-4">
            {lang === "es"
              ? "Esta sesión se desincronizó. Recargue esta página para obtener el estado más reciente de la entrevista."
              : "This session got out of sync. Reload this page to fetch the latest interview state."}
          </p>
          <Button fullWidth className="mb-3" onClick={onReloadPage}>
            {lang === "es" ? "Recargar página" : "Reload page"}
          </Button>
        </>
      ) : (
        <Link href={`/session/${code}`}>
          <Button fullWidth className="mb-3">
            {lang === "es" ? "Recuperar sesión con PIN" : "Recover session with PIN"}
          </Button>
        </Link>
      )}
      <Link href="/">
        <Button fullWidth variant="secondary">
          {lang === "es" ? "Volver al inicio" : "Back to home"}
        </Button>
      </Link>
    </>
  );
}
