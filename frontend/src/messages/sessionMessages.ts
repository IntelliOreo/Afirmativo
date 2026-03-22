import type { Lang } from "@/lib/language";
import type { VerifyResult } from "@/lib/sessionService";

type SessionCopy = {
  loading: string;
  networkError: string;
  genericAccessError: string;
  resumeHeading: string;
  resumeBody: string;
  readyHeading: string;
  readyBody: string;
  beginInterview: string;
  recoveryInfo: string;
  resumeButton: string;
  sessionNotFound: string;
  sessionExpired: string;
  pinInvalid: string;
  rateLimited: string;
  serverError: string;
  unknownError: string;
};

const SESSION_MESSAGES = {
  en: {
    loading: "Loading...",
    networkError: "Connection error. Please try again.",
    genericAccessError: "Could not verify the session. Please try again.",
    resumeHeading: "Resume session",
    resumeBody: "Enter your 6-digit PIN to resume access to your session.",
    readyHeading: "Your session is ready",
    readyBody: "You can start the interview right away using the button below.",
    beginInterview: "Begin interview",
    recoveryInfo: "If you lose your connection or want to come back later, use this info to resume your session. Save it or take a screenshot.",
    resumeButton: "Resume session",
    sessionNotFound: "Session code not found.",
    sessionExpired: "This session has expired.",
    pinInvalid: "Incorrect PIN. Please try again.",
    rateLimited: "Too many attempts. Please wait and try again.",
    serverError: "Could not verify the session right now. Please try again.",
    unknownError: "Could not verify the session. Please try again.",
  },
  es: {
    loading: "Cargando...",
    networkError: "Error de conexion. Intente de nuevo.",
    genericAccessError: "No se pudo verificar la sesion. Intente de nuevo.",
    resumeHeading: "Reanudar sesion",
    resumeBody: "Ingrese su PIN de 6 digitos para reanudar el acceso a su sesion.",
    readyHeading: "Su sesion esta lista",
    readyBody: "Puede comenzar la entrevista de inmediato con el boton de abajo.",
    beginInterview: "Comenzar entrevista",
    recoveryInfo: "Si pierde la conexion o desea volver mas tarde, use esta informacion para reanudar su sesion. Guardela o tome una captura de pantalla.",
    resumeButton: "Reanudar sesion",
    sessionNotFound: "Codigo de sesion no encontrado.",
    sessionExpired: "Esta sesion ha expirado.",
    pinInvalid: "PIN incorrecto. Intente de nuevo.",
    rateLimited: "Demasiados intentos. Espere un momento e intente otra vez.",
    serverError: "No se pudo verificar la sesion en este momento. Intente de nuevo.",
    unknownError: "No se pudo verificar la sesion. Intente de nuevo.",
  },
} as const satisfies Record<Lang, SessionCopy>;

export function getSessionMessages(lang: Lang) {
  return SESSION_MESSAGES[lang];
}

export function getSessionVerifyErrorMessage(
  lang: Lang,
  reason: Exclude<VerifyResult, { ok: true }>["reason"],
): string {
  const copy = getSessionMessages(lang);

  switch (reason) {
    case "not_found":
      return copy.sessionNotFound;
    case "expired":
      return copy.sessionExpired;
    case "invalid_pin":
      return copy.pinInvalid;
    case "rate_limited":
      return copy.rateLimited;
    case "network":
      return copy.networkError;
    case "server":
      return copy.serverError;
    default:
      return copy.unknownError;
  }
}

export { SESSION_MESSAGES };
