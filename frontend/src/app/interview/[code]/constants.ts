export const AUTOSUBMIT_SECONDS = 10; // auto-submit countdown threshold
export const WARNING_AT_SECONDS = 45 * 60; // orange bar at 45 min remaining
export const WRAPUP_AT_SECONDS = 5 * 60; // red bar + alert at 5 min remaining
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

export const VOICE_MAX_SECONDS = 180;
export const VOICE_WARNING_SECONDS = [120, 150, 170] as const;
export const VOICE_MIME_CANDIDATES = [
  "audio/webm;codecs=opus",
  "audio/webm",
  "audio/mp4",
  "audio/ogg;codecs=opus",
] as const;
export const VOICE_WAVE_BARS = [8, 14, 20, 12, 18, 24, 10, 16, 22, 14, 9, 19, 13, 21, 11] as const;
