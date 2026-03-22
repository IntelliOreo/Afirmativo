import type { Lang } from "@/lib/language";

const COMMON_MESSAGES = {
  en: {
    loading: "Loading...",
    verifying: "Verifying...",
    backHome: "Back to home",
    sessionCodeLabel: "Session code",
    linkLabel: "Link",
    pinLabel: "PIN",
    copied: "Copied",
    copyAll: "Copy all",
  },
  es: {
    loading: "Cargando...",
    verifying: "Verificando...",
    backHome: "Volver al inicio",
    sessionCodeLabel: "Codigo de sesion",
    linkLabel: "Enlace",
    pinLabel: "PIN",
    copied: "Copiado",
    copyAll: "Copiar todo",
  },
} as const satisfies Record<Lang, Record<string, string>>;

export function getCommonMessages(lang: Lang) {
  return COMMON_MESSAGES[lang];
}

export { COMMON_MESSAGES };
