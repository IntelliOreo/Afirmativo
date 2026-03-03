export type UILanguage = "es" | "en";

const langStorageKey = "ui_lang";

export function parseLang(value: string | null | undefined): UILanguage | null {
  if (value === "es" || value === "en") return value;
  return null;
}

export function readStoredLang(): UILanguage | null {
  if (typeof window === "undefined") return null;
  return parseLang(sessionStorage.getItem(langStorageKey));
}

export function writeStoredLang(lang: UILanguage): void {
  if (typeof window === "undefined") return;
  sessionStorage.setItem(langStorageKey, lang);
}

export function resolveLang(
  requested: string | null | undefined,
  fallback: UILanguage = "es",
): UILanguage {
  return parseLang(requested) ?? readStoredLang() ?? fallback;
}

export function withLang(path: string, lang: UILanguage): string {
  const separator = path.includes("?") ? "&" : "?";
  return `${path}${separator}lang=${lang}`;
}
