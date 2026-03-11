import { useCallback } from "react";
import { api } from "@/lib/api";
import * as pendingAnswerStore from "@/lib/storage/pendingAnswerStore";
import {
  ASYNC_POLL_BACKOFF_MS,
  ASYNC_POLL_CIRCUIT_BREAKER_COOLDOWN_MS,
  ASYNC_POLL_CIRCUIT_BREAKER_FAILURES,
  ASYNC_POLL_TIMEOUT_MS,
} from "../constants";
import type {
  AnswerAsyncAcceptedResponseDto,
  AnswerJobStatusResponseDto,
} from "../dto";
import { mapAnswerJobResponse, mapAsyncAcceptedResponse } from "../mappers";
import type { AnswerOutcome, PendingAnswerSubmission } from "../models";
import {
  buildCodedError,
  wait,
  withJitter,
} from "../utils";

interface UseAsyncAnswerPollingParams {
  code: string;
}

export function useAsyncAnswerPolling({ code }: UseAsyncAnswerPollingParams) {
  const pollAsyncAnswerJob = useCallback(async (
    jobId: string,
    isCanceled: () => boolean = () => false,
  ): Promise<AnswerOutcome> => {
    const startedAt = Date.now();
    let attempt = 0;
    let consecutiveTransientFailures = 0;

    while (true) {
      if (isCanceled()) {
        throw new Error("Polling canceled");
      }

      if (Date.now() - startedAt >= ASYNC_POLL_TIMEOUT_MS) {
        throw buildCodedError("Polling timed out before the answer job completed", "ASYNC_POLL_TIMEOUT");
      }

      try {
        const { ok, status: httpStatus, data } = await api<AnswerJobStatusResponseDto>(
          `/api/interview/answer-jobs/${encodeURIComponent(jobId)}?session_code=${encodeURIComponent(code)}`,
          { credentials: "include" },
        );

        if (!ok || !data) {
          const isTransientStatus = httpStatus === 429 || httpStatus >= 500;
          if (isTransientStatus) {
            consecutiveTransientFailures += 1;
            if (consecutiveTransientFailures >= ASYNC_POLL_CIRCUIT_BREAKER_FAILURES) {
              await wait(withJitter(ASYNC_POLL_CIRCUIT_BREAKER_COOLDOWN_MS));
              throw buildCodedError(
                "Polling paused after repeated server failures. Reload to retry.",
                "ASYNC_POLL_CIRCUIT_OPEN",
              );
            }

            const transientDelay = ASYNC_POLL_BACKOFF_MS[Math.min(attempt, ASYNC_POLL_BACKOFF_MS.length - 1)];
            attempt += 1;
            await wait(withJitter(transientDelay));
            continue;
          }

          throw buildCodedError(
            data?.error ?? (httpStatus === 404 ? "Async answer job not found" : "Failed to poll answer status"),
            data?.code,
          );
        }

        consecutiveTransientFailures = 0;
        const mapped = mapAnswerJobResponse(data);

        if (mapped.status === "queued" || mapped.status === "running") {
          const delay = ASYNC_POLL_BACKOFF_MS[Math.min(attempt, ASYNC_POLL_BACKOFF_MS.length - 1)];
          attempt += 1;
          await wait(withJitter(delay));
          continue;
        }

        if (mapped.status === "succeeded") {
          pendingAnswerStore.clear(code);
          return {
            done: mapped.done,
            nextQuestion: mapped.nextQuestion,
            timerRemainingS: mapped.timerRemainingS,
            answerSubmitWindowRemainingS: mapped.answerSubmitWindowRemainingS,
          };
        }

        if (mapped.status === "canceled") {
          throw buildCodedError(
            mapped.errorMessage || "Processing was canceled. Reload to continue.",
            mapped.errorCode || "AI_RETRY_EXHAUSTED",
          );
        }
        if (mapped.status === "conflict") {
          throw buildCodedError(
            mapped.errorMessage || "Turn is stale or out of order",
            mapped.errorCode || "TURN_CONFLICT",
          );
        }
        throw buildCodedError(
          mapped.errorMessage || mapped.errorCode || "Failed to process answer",
          mapped.errorCode,
        );
      } catch (err) {
        if (err instanceof TypeError) {
          consecutiveTransientFailures += 1;
          if (consecutiveTransientFailures >= ASYNC_POLL_CIRCUIT_BREAKER_FAILURES) {
            await wait(withJitter(ASYNC_POLL_CIRCUIT_BREAKER_COOLDOWN_MS));
            throw buildCodedError(
              "Polling paused after repeated network failures. Reload to retry.",
              "ASYNC_POLL_CIRCUIT_OPEN",
            );
          }

          const transientDelay = ASYNC_POLL_BACKOFF_MS[Math.min(attempt, ASYNC_POLL_BACKOFF_MS.length - 1)];
          attempt += 1;
          await wait(withJitter(transientDelay));
          continue;
        }

        throw err;
      }
    }
  }, [code]);

  const submitPendingAnswerJob = useCallback(async (
    pending: PendingAnswerSubmission,
    isCanceled: () => boolean = () => false,
  ): Promise<AnswerOutcome> => {
    let current = pending;
    let jobId = current.jobId;
    if (!jobId) {
      const { ok, data } = await api<AnswerAsyncAcceptedResponseDto>("/api/interview/answer-async", {
        method: "POST",
        body: {
          session_code: code,
          answer_text: current.answerText,
          question_text: current.questionText,
          turn_id: current.turnId,
          client_request_id: current.clientRequestId,
        },
        credentials: "include",
      });
      if (!ok || !data) {
        throw buildCodedError(data?.error ?? "Failed to queue answer", data?.code);
      }

      const accepted = mapAsyncAcceptedResponse(data);
      current = {
        ...current,
        clientRequestId: accepted.clientRequestId,
        jobId: accepted.jobId,
      };
      jobId = accepted.jobId;
      pendingAnswerStore.write(code, current);
    }

    return pollAsyncAnswerJob(jobId, isCanceled);
  }, [code, pollAsyncAnswerJob]);

  return { pollAsyncAnswerJob, submitPendingAnswerJob };
}
