"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import {
  ASYNC_POLL_BACKOFF_MS,
  ASYNC_POLL_CIRCUIT_BREAKER_COOLDOWN_MS,
  ASYNC_POLL_CIRCUIT_BREAKER_FAILURES,
  ASYNC_POLL_TIMEOUT_MS,
} from "../constants";
import type { InterviewReportDto } from "../dto";
import { mapReport } from "../mappers";
import type { InterviewReport } from "../models";
import type { ReportStatus } from "../viewTypes";
import { wait, withJitter } from "../utils";

interface ReportErrorResponse {
  error?: string;
  code?: string;
}

interface UseInterviewReportResult {
  reportStatus: ReportStatus;
  report: InterviewReport | null;
  reportError: string;
  resetReportState: () => void;
  loadReport: () => Promise<void>;
  resumeReport: () => Promise<void>;
  printReport: () => void;
}

function isReportNotStarted(status: number, data: ReportErrorResponse | null): boolean {
  return status === 404 && data?.code === "REPORT_NOT_STARTED";
}

export function useInterviewReport(code: string): UseInterviewReportResult {
  const [reportStatus, setReportStatus] = useState<ReportStatus>("idle");
  const [report, setReport] = useState<InterviewReport | null>(null);
  const [reportError, setReportError] = useState("");
  const pollGenerationRef = useRef(0);

  const resetReportState = useCallback(() => {
    pollGenerationRef.current += 1;
    setReportStatus("idle");
    setReport(null);
    setReportError("");
  }, []);

  const resumeReport = useCallback(async () => {
    setReportError("");

    try {
      const { ok, status: httpStatus, data } = await api<InterviewReportDto & ReportErrorResponse>(`/api/report/${code}`, {
        credentials: "include",
      });

      if (isReportNotStarted(httpStatus, data)) {
        setReportStatus("idle");
        setReport(null);
        return;
      }

      if (httpStatus === 202) {
        setReportStatus("generating");
        return;
      }

      if (!ok || !data) {
        throw new Error(data?.error ?? "Failed to load report");
      }

      setReport(mapReport(data));
      setReportStatus("ready");
    } catch (err) {
      setReportError(err instanceof Error ? err.message : "Unknown error");
      setReportStatus("error");
    }
  }, [code]);

  const loadReport = useCallback(async () => {
    setReportError("");
    setReportStatus("loading");

    try {
      const { ok, status: httpStatus, data } = await api<InterviewReportDto & ReportErrorResponse>(
        `/api/report/${code}/generate`,
        {
          method: "POST",
          credentials: "include",
        },
      );

      if (httpStatus === 202) {
        setReportStatus("generating");
        return;
      }

      if (!ok || !data) {
        throw new Error(data?.error ?? "Failed to queue report");
      }

      setReport(mapReport(data));
      setReportStatus("ready");
    } catch (err) {
      setReportError(err instanceof Error ? err.message : "Unknown error");
      setReportStatus("error");
    }
  }, [code]);

  useEffect(() => {
    if (reportStatus !== "generating") {
      pollGenerationRef.current += 1
      return;
    }

    const generation = pollGenerationRef.current + 1;
    pollGenerationRef.current = generation;
    let canceled = false;

    void (async () => {
      const startedAt = Date.now();
      let attempt = 0;
      let consecutiveTransientFailures = 0;

      while (!canceled) {
        if (Date.now() - startedAt >= ASYNC_POLL_TIMEOUT_MS) {
          setReportError("Report polling timed out. Try again.");
          setReportStatus("error");
          return;
        }

        try {
          const { ok, status: httpStatus, data } = await api<InterviewReportDto & ReportErrorResponse>(
            `/api/report/${code}`,
            { credentials: "include" },
          );
          if (canceled || pollGenerationRef.current !== generation) return;

          if (httpStatus === 202) {
            consecutiveTransientFailures = 0;
            const delay = ASYNC_POLL_BACKOFF_MS[Math.min(attempt, ASYNC_POLL_BACKOFF_MS.length - 1)];
            attempt += 1;
            await wait(withJitter(delay));
            continue;
          }

          if (isReportNotStarted(httpStatus, data)) {
            setReportStatus("idle");
            setReport(null);
            return;
          }

          if (!ok || !data) {
            const isTransientStatus = httpStatus === 429 || httpStatus >= 500;
            if (isTransientStatus && data?.code !== "GENERATION_FAILED") {
              consecutiveTransientFailures += 1;
              if (consecutiveTransientFailures >= ASYNC_POLL_CIRCUIT_BREAKER_FAILURES) {
                await wait(withJitter(ASYNC_POLL_CIRCUIT_BREAKER_COOLDOWN_MS));
                setReportError("Report polling paused after repeated failures. Try again.");
                setReportStatus("error");
                return;
              }

              const transientDelay = ASYNC_POLL_BACKOFF_MS[Math.min(attempt, ASYNC_POLL_BACKOFF_MS.length - 1)];
              attempt += 1;
              await wait(withJitter(transientDelay));
              continue;
            }

            throw new Error(data?.error ?? "Failed to load report");
          }

          setReport(mapReport(data));
          setReportStatus("ready");
          return;
        } catch (err) {
          if (canceled || pollGenerationRef.current !== generation) return;
          if (err instanceof TypeError) {
            consecutiveTransientFailures += 1;
            if (consecutiveTransientFailures >= ASYNC_POLL_CIRCUIT_BREAKER_FAILURES) {
              await wait(withJitter(ASYNC_POLL_CIRCUIT_BREAKER_COOLDOWN_MS));
              setReportError("Report polling paused after repeated network failures. Try again.");
              setReportStatus("error");
              return;
            }

            const transientDelay = ASYNC_POLL_BACKOFF_MS[Math.min(attempt, ASYNC_POLL_BACKOFF_MS.length - 1)];
            attempt += 1;
            await wait(withJitter(transientDelay));
            continue;
          }

          setReportError(err instanceof Error ? err.message : "Unknown error");
          setReportStatus("error");
          return;
        }
      }
    })();

    return () => {
      canceled = true;
    };
  }, [code, reportStatus]);

  const printReport = useCallback(() => {
    if (typeof window === "undefined") return;
    window.print();
  }, []);

  return {
    reportStatus,
    report,
    reportError,
    resetReportState,
    loadReport,
    resumeReport,
    printReport,
  };
}
