"use client";

import { WAITING_STATUS_MESSAGES } from "../messages/waitingStatusMessages";
import type { Lang } from "@/lib/language";
import type { InterviewPhase, ReportStatus, SubmitMode } from "../viewTypes";
import { useRotatingStatus } from "./useRotatingStatus";

interface UseInterviewWaitingStatusParams {
  lang: Lang;
  phase: InterviewPhase;
  submitMode: SubmitMode | null;
  reportStatus: ReportStatus;
}

interface UseInterviewWaitingStatusResult {
  startupWaitStatus: string;
  questionWaitStatus: string;
  reportWaitStatus: string;
}

export function useInterviewWaitingStatus({
  lang,
  phase,
  submitMode,
  reportStatus,
}: UseInterviewWaitingStatusParams): UseInterviewWaitingStatusResult {
  const waitCopy = WAITING_STATUS_MESSAGES[lang];

  const startupWaitStatus = useRotatingStatus(waitCopy.startup, phase === "loading");
  const questionWaitStatus = useRotatingStatus(
    waitCopy.question,
    phase === "submitting" && submitMode === "question",
  );
  const reportWaitStatus = useRotatingStatus(
    waitCopy.report,
    reportStatus === "loading" || reportStatus === "generating",
  );

  return {
    startupWaitStatus,
    questionWaitStatus,
    reportWaitStatus,
  };
}
