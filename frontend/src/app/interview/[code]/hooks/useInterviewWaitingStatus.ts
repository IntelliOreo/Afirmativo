"use client";

import { WAITING_STATUS_MESSAGES } from "../messages/waitingStatusMessages";
import type { InterviewStatus, Lang, ReportStatus } from "../types";
import { useRotatingStatus } from "./useRotatingStatus";

interface UseInterviewWaitingStatusParams {
  lang: Lang;
  interviewStatus: InterviewStatus;
  isSubmittingInQuestionFlow: boolean;
  forceSubmit: boolean;
  reportStatus: ReportStatus;
}

interface UseInterviewWaitingStatusResult {
  startupWaitStatus: string;
  questionWaitStatus: string;
  finalSubmitWaitStatus: string;
  reportWaitStatus: string;
}

export function useInterviewWaitingStatus({
  lang,
  interviewStatus,
  isSubmittingInQuestionFlow,
  forceSubmit,
  reportStatus,
}: UseInterviewWaitingStatusParams): UseInterviewWaitingStatusResult {
  const waitCopy = WAITING_STATUS_MESSAGES[lang];

  const startupWaitStatus = useRotatingStatus(waitCopy.startup, interviewStatus === "loading");
  const questionWaitStatus = useRotatingStatus(waitCopy.question, isSubmittingInQuestionFlow);
  const finalSubmitWaitStatus = useRotatingStatus(
    waitCopy.finalSubmit,
    interviewStatus === "submitting" && forceSubmit,
  );
  const reportWaitStatus = useRotatingStatus(
    waitCopy.report,
    reportStatus === "loading" || reportStatus === "generating",
  );

  return {
    startupWaitStatus,
    questionWaitStatus,
    finalSubmitWaitStatus,
    reportWaitStatus,
  };
}
