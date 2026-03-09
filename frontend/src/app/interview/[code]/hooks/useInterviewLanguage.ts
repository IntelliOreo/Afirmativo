"use client";

import type { Dispatch, SetStateAction } from "react";
import { useLanguage } from "@/lib/useLanguage";
import type { Lang } from "@/lib/language";

interface UseInterviewLanguageResult {
  lang: Lang;
  setLang: Dispatch<SetStateAction<Lang>>;
  langInitialized: boolean;
}

export function useInterviewLanguage(
  code: string,
  requestedLang: string | null,
): UseInterviewLanguageResult {
  const { lang, setLang, initialized } = useLanguage({ requestedLang, sessionCode: code });
  return { lang, setLang: setLang as Dispatch<SetStateAction<Lang>>, langInitialized: initialized };
}
