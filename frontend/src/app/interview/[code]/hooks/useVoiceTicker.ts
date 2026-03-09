"use client";

import { useCallback, useEffect, useRef, useState } from "react";

interface UseVoiceTickerParams {
  maxSeconds: number;
  warningMilestones: readonly number[];
  onLimitReached: () => void;
}

interface UseVoiceTickerResult {
  durationSeconds: number;
  warningSeconds: number | null;
  start: () => void;
  pause: () => number;
  resume: () => void;
  stop: () => number;
  reset: () => void;
}

export function useVoiceTicker({
  maxSeconds,
  warningMilestones,
  onLimitReached,
}: UseVoiceTickerParams): UseVoiceTickerResult {
  const [durationSeconds, setDurationSeconds] = useState(0);
  const [warningSeconds, setWarningSeconds] = useState<number | null>(null);

  const tickerRef = useRef<number | null>(null);
  const startedAtMsRef = useRef(0);
  const elapsedMsRef = useRef(0);
  const durationRef = useRef(0);

  const clearTicker = useCallback(() => {
    if (tickerRef.current !== null) {
      window.clearInterval(tickerRef.current);
      tickerRef.current = null;
    }
  }, []);

  const applyElapsedMs = useCallback((elapsedMs: number): number => {
    const clampedElapsedMs = Math.min(maxSeconds * 1000, Math.max(0, elapsedMs));
    const elapsedSeconds = Math.min(maxSeconds, Math.floor(clampedElapsedMs / 1000));

    if (elapsedSeconds !== durationRef.current) {
      durationRef.current = elapsedSeconds;
      setDurationSeconds(elapsedSeconds);
      if (warningMilestones.includes(elapsedSeconds)) {
        setWarningSeconds(elapsedSeconds);
      }
    }

    return elapsedSeconds;
  }, [maxSeconds, warningMilestones]);

  const stop = useCallback((): number => {
    let elapsedMs = elapsedMsRef.current;
    if (startedAtMsRef.current > 0) {
      elapsedMs += Date.now() - startedAtMsRef.current;
    }

    elapsedMsRef.current = Math.min(maxSeconds * 1000, Math.max(0, elapsedMs));
    startedAtMsRef.current = 0;
    clearTicker();
    return applyElapsedMs(elapsedMsRef.current);
  }, [applyElapsedMs, clearTicker, maxSeconds]);

  const pause = useCallback((): number => stop(), [stop]);

  const tick = useCallback(() => {
    if (startedAtMsRef.current <= 0) return;

    const elapsedMs = elapsedMsRef.current + (Date.now() - startedAtMsRef.current);
    const elapsedSeconds = applyElapsedMs(elapsedMs);
    if (elapsedSeconds >= maxSeconds) {
      elapsedMsRef.current = maxSeconds * 1000;
      startedAtMsRef.current = 0;
      clearTicker();
      onLimitReached();
    }
  }, [applyElapsedMs, clearTicker, maxSeconds, onLimitReached]);

  const startTicker = useCallback(() => {
    clearTicker();
    tickerRef.current = window.setInterval(tick, 250);
  }, [clearTicker, tick]);

  const start = useCallback(() => {
    clearTicker();
    startedAtMsRef.current = Date.now();
    elapsedMsRef.current = 0;
    durationRef.current = 0;
    setDurationSeconds(0);
    setWarningSeconds(null);
    startTicker();
  }, [clearTicker, startTicker]);

  const resume = useCallback(() => {
    if (startedAtMsRef.current > 0) return;
    startedAtMsRef.current = Date.now();
    startTicker();
  }, [startTicker]);

  const reset = useCallback(() => {
    clearTicker();
    startedAtMsRef.current = 0;
    elapsedMsRef.current = 0;
    durationRef.current = 0;
    setDurationSeconds(0);
    setWarningSeconds(null);
  }, [clearTicker]);

  useEffect(() => {
    return () => {
      clearTicker();
    };
  }, [clearTicker]);

  return {
    durationSeconds,
    warningSeconds,
    start,
    pause,
    resume,
    stop,
    reset,
  };
}
