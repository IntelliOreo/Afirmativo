"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { RefObject } from "react";
import type { Question } from "../models";

interface UseDisclaimerScrollGateResult {
  disclaimerScrollRef: RefObject<HTMLDivElement | null>;
  hasReachedDisclaimerBottom: boolean;
  updateDisclaimerScrollState: () => void;
}

export function useDisclaimerScrollGate(
  question: Question | null,
): UseDisclaimerScrollGateResult {
  const disclaimerScrollRef = useRef<HTMLDivElement | null>(null);
  const [hasReachedDisclaimerBottom, setHasReachedDisclaimerBottom] = useState(false);

  const updateDisclaimerScrollState = useCallback(() => {
    const el = disclaimerScrollRef.current;
    if (!el) return;

    const noScrollNeeded = el.scrollHeight <= el.clientHeight + 4;
    const atBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 4;
    if (noScrollNeeded || atBottom) {
      setHasReachedDisclaimerBottom(true);
    }
  }, []);

  useEffect(() => {
    if (question?.kind === "disclaimer") {
      setHasReachedDisclaimerBottom(false);
    }
  }, [question?.kind]);

  useEffect(() => {
    if (question?.kind !== "disclaimer") return;

    const id = window.requestAnimationFrame(updateDisclaimerScrollState);
    return () => window.cancelAnimationFrame(id);
  }, [question?.kind, question?.textEn, question?.textEs, updateDisclaimerScrollState]);

  return {
    disclaimerScrollRef,
    hasReachedDisclaimerBottom,
    updateDisclaimerScrollState,
  };
}
