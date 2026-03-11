"use client";

import { Alert } from "@components/Alert";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import type { Lang } from "@/lib/language";
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

function getDialogCopy(lang: Lang, mode: MicrophoneWarmupDialogMode, uiState: MicrophoneWarmupDialogState) {
  const isSpanish = lang === "es";

  if (uiState === "ready_handoff") {
    return {
      eyebrow: isSpanish ? "Micrófono listo" : "Microphone ready",
      title: isSpanish ? "Su micrófono ya está preparado" : "Your microphone is ready",
      body: isSpanish
        ? "Terminando la conexión para que la entrevista continúe sin interrupciones."
        : "Finishing the connection so the interview can continue without interruption.",
      status: isSpanish ? "Micrófono conectado correctamente." : "Microphone connected successfully.",
      statusVariant: "success" as const,
      buttonLabel: isSpanish ? "Conectando..." : "Connecting...",
    };
  }

  if (uiState === "warming") {
    return {
      eyebrow: isSpanish ? "Configurando micrófono" : "Preparing microphone",
      title: isSpanish ? "Preparando el micrófono" : "Preparing the microphone",
      body: isSpanish
        ? "La primera conexión puede tardar un poco. Mantenga esta ventana abierta mientras terminamos."
        : "The first connection can take a moment. Keep this window open while we finish.",
      status: isSpanish ? "Conectando el micrófono. Esto puede tardar un momento." : "Connecting the microphone. This can take a moment.",
      statusVariant: "info" as const,
      buttonLabel: isSpanish ? "Conectando..." : "Connecting...",
    };
  }

  if (uiState === "recovering") {
    return {
      eyebrow: isSpanish ? "Reconectando micrófono" : "Reconnecting microphone",
      title: isSpanish ? "Volviendo a preparar el micrófono" : "Preparing the microphone again",
      body: isSpanish
        ? "La conexión del micrófono se interrumpió. Estamos intentando recuperarla antes de continuar."
        : "The microphone connection was interrupted. We are restoring it before continuing.",
      status: isSpanish ? "Reconectando el micrófono." : "Reconnecting the microphone.",
      statusVariant: "warning" as const,
      buttonLabel: isSpanish ? "Reconectando..." : "Reconnecting...",
    };
  }

  if (uiState === "denied") {
    return {
      eyebrow: isSpanish ? "Permiso requerido" : "Permission required",
      title: isSpanish ? "Necesitamos acceso al micrófono" : "We need microphone access",
      body: isSpanish
        ? "Puede intentarlo otra vez cuando quiera responder por voz."
        : "You can try again whenever you want to answer by voice.",
      status: isSpanish
        ? "No se concedió permiso al micrófono. Puede volver a intentarlo."
        : "Microphone permission was not granted. You can try again.",
      statusVariant: "error" as const,
      buttonLabel: isSpanish ? "Habilitar micrófono" : "Enable microphone",
    };
  }

  if (uiState === "error") {
    return {
      eyebrow: isSpanish ? "Micrófono no disponible" : "Microphone unavailable",
      title: isSpanish ? "No pudimos preparar el micrófono" : "We could not prepare the microphone",
      body: isSpanish
        ? "Inténtelo otra vez. Si el problema continúa, puede seguir con texto por ahora."
        : "Try again. If the problem continues, you can keep going with text for now.",
      status: isSpanish
        ? "El micrófono necesita reconectarse antes de la próxima grabación."
        : "The microphone needs to reconnect before the next recording.",
      statusVariant: "error" as const,
      buttonLabel: isSpanish
        ? (mode === "reconnect" ? "Reconectar micrófono" : "Reintentar micrófono")
        : (mode === "reconnect" ? "Reconnect microphone" : "Retry microphone"),
    };
  }

  return {
    eyebrow: isSpanish ? "Micrófono opcional" : "Optional microphone setup",
    title: isSpanish ? "Prepare el micrófono ahora" : "Prepare the microphone now",
    body: isSpanish
      ? "Si piensa responder por voz, este es el mejor momento para conceder permiso al micrófono. Una vez habilitado, el indicador del navegador puede permanecer encendido hasta la etapa del reporte."
      : "If you plan to answer by voice, this is the best time to grant microphone permission. Once enabled, the browser indicator may stay on until the report stage.",
    status: isSpanish
      ? "Si piensa responder por voz, habilite el micrófono ahora para evitar demora cuando grabe."
      : "If you plan to answer by voice, enable the microphone now to avoid delay when you record.",
    statusVariant: "info" as const,
    buttonLabel: isSpanish ? "Habilitar micrófono" : "Enable microphone",
  };
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
  const copy = getDialogCopy(lang, mode, uiState);
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
            {mode === "reconnect"
              ? (lang === "es" ? "Cerrar" : "Close")
              : (lang === "es" ? "Ahora no" : "Not now")}
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
