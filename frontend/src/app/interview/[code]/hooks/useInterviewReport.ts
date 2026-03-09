"use client";

import { useCallback, useState } from "react";
import { api } from "@/lib/api";
import type { InterviewReportDto } from "../dto";
import { mapReport } from "../mappers";
import type { InterviewReport } from "../models";
import type { ReportStatus } from "../viewTypes";

interface UseInterviewReportResult {
  reportStatus: ReportStatus;
  report: InterviewReport | null;
  reportError: string;
  resetReportState: () => void;
  loadReport: () => Promise<void>;
  printReport: () => void;
}

export function useInterviewReport(code: string): UseInterviewReportResult {
  const [reportStatus, setReportStatus] = useState<ReportStatus>("idle");
  const [report, setReport] = useState<InterviewReport | null>(null);
  const [reportError, setReportError] = useState("");

  const resetReportState = useCallback(() => {
    setReportStatus("idle");
    setReport(null);
    setReportError("");
  }, []);

  const loadReport = useCallback(async () => {
    setReportError("");
    setReportStatus("loading");

    try {
      const { ok, status: httpStatus, data } = await api<InterviewReportDto & { error?: string }>(`/api/report/${code}`, {
        credentials: "include",
      });

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
    printReport,
  };
}
