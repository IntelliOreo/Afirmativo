import type { Lang } from "@/lib/language";

export const AUTOSUBMIT_SECONDS = 10; // auto-submit countdown threshold
export const WARNING_AT_SECONDS = 45 * 60; // orange bar at 45 min remaining
export const WRAPUP_AT_SECONDS = 5 * 60; // red bar + alert at 5 min remaining
export const TEXT_ANSWER_MAX_CHARS = 3000;
export const ASYNC_POLL_BACKOFF_MS = [1000, 2000, 3000, 5000, 8000, 15000, 20000, 30000] as const;

function parsePositiveIntEnv(rawValue: string | undefined, fallback: number): number {
  const parsed = Number.parseInt((rawValue ?? "").trim(), 10);
  if (!Number.isFinite(parsed) || parsed <= 0) return fallback;
  return parsed;
}

const asyncPollTimeoutSeconds = parsePositiveIntEnv(
  process.env.NEXT_PUBLIC_ASYNC_POLL_TIMEOUT_SECONDS,
  180,
);
const asyncPollCircuitBreakerFailures = parsePositiveIntEnv(
  process.env.NEXT_PUBLIC_ASYNC_POLL_CIRCUIT_BREAKER_FAILURES,
  3,
);
const asyncPollCircuitBreakerCooldownSeconds = parsePositiveIntEnv(
  process.env.NEXT_PUBLIC_ASYNC_POLL_CIRCUIT_BREAKER_COOLDOWN_SECONDS,
  15,
);

export const ASYNC_POLL_TIMEOUT_MS = asyncPollTimeoutSeconds * 1000;
export const ASYNC_POLL_CIRCUIT_BREAKER_FAILURES = asyncPollCircuitBreakerFailures;
export const ASYNC_POLL_CIRCUIT_BREAKER_COOLDOWN_MS = asyncPollCircuitBreakerCooldownSeconds * 1000;

function buildVoiceWarningSeconds(maxSeconds: number): number[] {
  const warningOffsets = [60, 30, 10];
  const seen = new Set<number>();

  return warningOffsets
    .map((offset) => maxSeconds - offset)
    .filter((second) => second > 0 && second < maxSeconds)
    .filter((second) => {
      if (seen.has(second)) return false;
      seen.add(second);
      return true;
    })
    .sort((left, right) => left - right);
}

export function formatDurationLabel(seconds: number, lang: Lang): string {
  if (seconds % 60 === 0) {
    const minutes = seconds / 60;
    if (lang === "es") {
      return `${minutes} ${minutes === 1 ? "minuto" : "minutos"}`;
    }
    return `${minutes} ${minutes === 1 ? "minute" : "minutes"}`;
  }

  if (lang === "es") {
    return `${seconds} ${seconds === 1 ? "segundo" : "segundos"}`;
  }
  return `${seconds} ${seconds === 1 ? "second" : "seconds"}`;
}

export const VOICE_MAX_SECONDS = parsePositiveIntEnv(
  process.env.NEXT_PUBLIC_VOICE_MAX_SECONDS,
  180,
);
export const VOICE_WARNING_SECONDS = buildVoiceWarningSeconds(VOICE_MAX_SECONDS);
export const VOICE_MIME_CANDIDATES = [
  "audio/webm;codecs=opus",
  "audio/webm",
  "audio/mp4",
  "audio/ogg;codecs=opus",
] as const;
export const VOICE_WAVE_BARS = [8, 14, 20, 12, 18, 24, 10, 16, 22, 14, 9, 19, 13, 21, 11] as const;
