export type Lang = "es" | "en";
export type UILanguage = Lang;

export function parseLang(value: string | null | undefined): Lang | null {
  if (value === "es" || value === "en") return value;
  return null;
}

export function withLang(path: string, lang: Lang): string {
  const separator = path.includes("?") ? "&" : "?";
  return `${path}${separator}lang=${lang}`;
}
