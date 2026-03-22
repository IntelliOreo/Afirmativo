"use client";

import { useEffect, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import { type Lang, parseLang } from "@/lib/language";
import {
  readInterviewLang,
  readUiLang,
  writeInterviewLang,
  writeUiLang,
} from "@/lib/storage/languageStore";

interface UseLanguageParams {
  requestedLang: string | null;
  sessionCode?: string;
}

interface UseLanguageResult {
  lang: Lang;
  setLang: Dispatch<SetStateAction<Lang>>;
  initialized: boolean;
}

export function useLanguage({
  requestedLang,
  sessionCode,
}: UseLanguageParams): UseLanguageResult {
  const [lang, setLang] = useState<Lang>("es");
  const [initialized, setInitialized] = useState(false);

  useEffect(() => {
    const langFromQuery = parseLang(requestedLang);
    if (langFromQuery) {
      setLang(langFromQuery);
      setInitialized(true);
      return;
    }

    if (sessionCode) {
      const interviewLang = readInterviewLang(sessionCode);
      if (interviewLang) {
        setLang(interviewLang);
        setInitialized(true);
        return;
      }
    }

    const uiLang = readUiLang();
    if (uiLang) {
      setLang(uiLang);
    }

    setInitialized(true);
  }, [requestedLang, sessionCode]);

  useEffect(() => {
    if (!initialized) return;

    writeUiLang(lang);
    if (sessionCode) {
      writeInterviewLang(sessionCode, lang);
    }
  }, [initialized, lang, sessionCode]);

  return { lang, setLang, initialized };
}
