const LEVELS = { debug: 0, info: 1, warn: 2, error: 3 } as const;
type Level = keyof typeof LEVELS;

const currentLevel: Level =
  (process.env.NEXT_PUBLIC_LOG_LEVEL as Level) || "info";

function shouldLog(level: Level): boolean {
  return LEVELS[level] >= LEVELS[currentLevel];
}

function fmt(level: Level, msg: string, data?: Record<string, unknown>): string {
  const ts = new Date().toISOString();
  const extra = data ? " " + JSON.stringify(data) : "";
  return `[${ts}] ${level.toUpperCase()} ${msg}${extra}`;
}

export const log = {
  debug(msg: string, data?: Record<string, unknown>) {
    if (shouldLog("debug")) console.debug(fmt("debug", msg, data));
  },
  info(msg: string, data?: Record<string, unknown>) {
    if (shouldLog("info")) console.info(fmt("info", msg, data));
  },
  warn(msg: string, data?: Record<string, unknown>) {
    if (shouldLog("warn")) console.warn(fmt("warn", msg, data));
  },
  error(msg: string, data?: Record<string, unknown>) {
    if (shouldLog("error")) console.error(fmt("error", msg, data));
  },
};
