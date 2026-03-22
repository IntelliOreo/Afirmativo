"use client";

import { useEffect, useState } from "react";
import {
  WAITING_STATUS_INITIAL_DELAY_MS,
  WAITING_STATUS_ROTATION_MS,
} from "../messages/waitingStatusMessages";
import { randomMessageIndex } from "../utils";

interface UseRotatingStatusOptions {
  intervalMs?: number;
  initialDelayMs?: number;
}

export function useRotatingStatus(
  messages: readonly string[],
  active: boolean,
  options?: UseRotatingStatusOptions,
): string {
  const [index, setIndex] = useState(0);
  const [visible, setVisible] = useState(false);
  const intervalMs = options?.intervalMs ?? WAITING_STATUS_ROTATION_MS;
  const initialDelayMs = options?.initialDelayMs ?? WAITING_STATUS_INITIAL_DELAY_MS;

  useEffect(() => {
    if (!active || messages.length === 0) {
      setIndex(0);
      setVisible(false);
      return;
    }

    setIndex(0);
    setVisible(false);

    let interval: number | undefined;
    const delay = window.setTimeout(() => {
      setIndex(() => randomMessageIndex(-1, messages.length));
      setVisible(true);
      interval = window.setInterval(() => {
        setIndex((currentIndex) => randomMessageIndex(currentIndex, messages.length));
      }, intervalMs);
    }, initialDelayMs);

    return () => {
      window.clearTimeout(delay);
      if (interval !== undefined) {
        window.clearInterval(interval);
      }
    };
  }, [active, initialDelayMs, intervalMs, messages]);

  return visible ? (messages[index] ?? "") : "";
}
