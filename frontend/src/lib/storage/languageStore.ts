import { parseLang, type UILanguage } from "@/lib/language";

const UI_LANG_KEY = "ui_lang";

function interviewLangKey(sessionCode: string): string {
  return `interview_lang_${sessionCode}`;
}

export function readUiLang(): UILanguage | null {
  if (typeof window === "undefined") return null;
  return parseLang(sessionStorage.getItem(UI_LANG_KEY));
}

export function writeUiLang(lang: UILanguage): void {
  if (typeof window === "undefined") return;
  sessionStorage.setItem(UI_LANG_KEY, lang);
}

export function readInterviewLang(sessionCode: string): UILanguage | null {
  if (typeof window === "undefined") return null;
  return parseLang(sessionStorage.getItem(interviewLangKey(sessionCode)));
}

export function writeInterviewLang(sessionCode: string, lang: UILanguage): void {
  if (typeof window === "undefined") return;
  sessionStorage.setItem(interviewLangKey(sessionCode), lang);
}
