import type { Lang } from "@/lib/language";
import type { VerifyResult } from "@/lib/sessionService";

type LandingCopy = {
  headline: string;
  subheadline: string;
  steps: readonly string[];
  cta: string;
  note: string;
  adminLink: string;
  resumeHeading: string;
  resumeBody: string;
  resumeButton: string;
  pinInvalid: string;
  sessionNotFound: string;
  sessionExpired: string;
  rateLimited: string;
  networkError: string;
  serverError: string;
  unknownError: string;
};

export const LANDING_TESTIMONIALS = [
  {
    author: "G.",
    quote:
      "“Practicar esto de verdad ha hecho una gran diferencia. Explicar lo que pasó ahora me sale mucho más claro y natural. Antes era un punto débil, pero ha mejorado bastante.”",
  },
  {
    author: "J.",
    quote:
      "“Gracias, ya llevo más de diez sesiones practicando y, de verdad, me ha ayudado muchísimo a bajar la ansiedad.”",
  },
] as const;

const LANDING_MESSAGES = {
  en: {
    headline: "Prepare for your affirmative asylum interview",
    subheadline: "A confidential practice tool with a bilingual assessment report.",
    steps: [
      "Read Before You Start and agree to the terms",
      "Enter a coupon code or pay online",
      "Complete the simulated interview - one question at a time",
      "Download your bilingual assessment report",
    ],
    cta: "Get Started",
    note: "No account. No login. Your session is completely anonymous.",
    adminLink: "Admin (dev): DB cleanup",
    resumeHeading: "Already have a session?",
    resumeBody: "Enter your session code and PIN to continue.",
    resumeButton: "Resume session",
    pinInvalid: "Incorrect PIN. Please try again.",
    sessionNotFound: "Session code not found.",
    sessionExpired: "This session has expired.",
    rateLimited: "Too many attempts. Please wait and try again.",
    networkError: "Connection error. Please try again.",
    serverError: "Could not verify the session right now. Please try again.",
    unknownError: "Could not verify the session. Please try again.",
  },
  es: {
    headline: "Preparese para su entrevista afirmativa de asilo",
    subheadline: "Una herramienta confidencial de practica con reporte bilingue.",
    steps: [
      "Lea Antes de Comenzar y acepte los terminos",
      "Ingrese un cupon o pague en linea",
      "Realice la entrevista simulada - una pregunta a la vez",
      "Descargue su reporte bilingue de evaluacion",
    ],
    cta: "Comenzar",
    note: "Sin cuenta. Sin inicio de sesion. Su sesion es completamente anonima.",
    adminLink: "Admin (dev): limpieza DB",
    resumeHeading: "Ya tiene una sesion?",
    resumeBody: "Ingrese su codigo de sesion y PIN para continuar.",
    resumeButton: "Reanudar sesion",
    pinInvalid: "PIN incorrecto. Intente de nuevo.",
    sessionNotFound: "Codigo de sesion no encontrado.",
    sessionExpired: "Esta sesion ha expirado.",
    rateLimited: "Demasiados intentos. Espere un momento e intente otra vez.",
    networkError: "Error de conexion. Intente de nuevo.",
    serverError: "No se pudo verificar la sesion en este momento. Intente de nuevo.",
    unknownError: "No se pudo verificar la sesion. Intente de nuevo.",
  },
} as const satisfies Record<Lang, LandingCopy>;

export function getLandingMessages(lang: Lang) {
  return LANDING_MESSAGES[lang];
}

export function getLandingVerifyErrorMessage(
  lang: Lang,
  reason: Exclude<VerifyResult, { ok: true }>["reason"],
): string {
  const copy = getLandingMessages(lang);

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

export { LANDING_MESSAGES };
