import { renderHook } from "@testing-library/react";
import { act } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiTimeoutError } from "@/lib/api";
import type { PendingAnswerSubmission } from "../models";
import { toClientRequestId, toJobId, toTurnId } from "../models";
import type { AnswerAsyncAcceptedResponseDto, AnswerJobStatusResponseDto } from "../dto";
import { useAsyncAnswerPolling } from "./useAsyncAnswerPolling";

const apiMock = vi.fn();
const pendingAnswerStoreClearMock = vi.fn();
const pendingAnswerStoreWriteMock = vi.fn();

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return {
    ...actual,
    api: (...args: unknown[]) => apiMock(...args),
  };
});

vi.mock("@/lib/storage/pendingAnswerStore", () => ({
  clear: (...args: unknown[]) => pendingAnswerStoreClearMock(...args),
  write: (...args: unknown[]) => pendingAnswerStoreWriteMock(...args),
  read: vi.fn(() => null),
}));

vi.mock("../utils", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../utils")>();
  return {
    ...actual,
    wait: vi.fn(() => Promise.resolve()),
    withJitter: (ms: number) => ms,
  };
});

const CODE = "AP-TEST";

function makePending(overrides: Partial<PendingAnswerSubmission> = {}): PendingAnswerSubmission {
  return {
    clientRequestId: toClientRequestId("crid-1"),
    turnId: toTurnId("turn-1"),
    answerText: "My answer",
    questionText: "The question",
    ...overrides,
  };
}

function makeAcceptedResponse(jobId = "job-1"): { ok: true; status: 200; data: AnswerAsyncAcceptedResponseDto } {
  return {
    ok: true,
    status: 200,
    data: {
      job_id: jobId,
      client_request_id: "crid-1",
      status: "queued",
    },
  };
}

function makeJobResponse(
  status: AnswerJobStatusResponseDto["status"],
  overrides: Partial<AnswerJobStatusResponseDto> = {},
): { ok: true; status: 200; data: AnswerJobStatusResponseDto } {
  return {
    ok: true,
    status: 200,
    data: {
      job_id: "job-1",
      client_request_id: "crid-1",
      status,
      done: false,
      timer_remaining_s: 500,
      answer_submit_window_remaining_s: 200,
      next_question: {
        text_es: "Siguiente",
        text_en: "Next",
        area: "area_1",
        kind: "criterion",
        turn_id: "turn-2",
        question_number: 3,
        total_questions: 25,
      },
      ...overrides,
    },
  };
}

function makeErrorResponse(httpStatus: number, error?: string, code?: string) {
  return {
    ok: false,
    status: httpStatus,
    data: { error, code },
  };
}

describe("useAsyncAnswerPolling", () => {
  beforeEach(() => {
    apiMock.mockReset();
    pendingAnswerStoreClearMock.mockReset();
    pendingAnswerStoreWriteMock.mockReset();
  });

  describe("submitPendingAnswerJob", () => {
    it("submits answer via POST, polls job, returns outcome on success", async () => {
      apiMock
        .mockResolvedValueOnce(makeAcceptedResponse("job-1"))
        .mockResolvedValueOnce(makeJobResponse("succeeded"));

      const { result } = renderHook(() => useAsyncAnswerPolling({ code: CODE }));

      let outcome: unknown;
      await act(async () => {
        outcome = await result.current.submitPendingAnswerJob(makePending());
      });

      expect(apiMock).toHaveBeenCalledTimes(2);
      expect(apiMock.mock.calls[0][0]).toBe("/api/interview/answer-async");
      expect(apiMock.mock.calls[1][0]).toContain("/api/interview/answer-jobs/job-1");
      expect(outcome).toMatchObject({ done: false, timerRemainingS: 500 });
    });

    it("resumes polling from existing jobId without re-posting", async () => {
      apiMock.mockResolvedValueOnce(makeJobResponse("succeeded"));

      const { result } = renderHook(() => useAsyncAnswerPolling({ code: CODE }));

      await act(async () => {
        await result.current.submitPendingAnswerJob(
          makePending({ jobId: toJobId("existing-job") }),
        );
      });

      expect(apiMock).toHaveBeenCalledTimes(1);
      expect(apiMock.mock.calls[0][0]).toContain("/api/interview/answer-jobs/existing-job");
    });

    it("writes updated pending job with jobId after initial POST acceptance", async () => {
      apiMock
        .mockResolvedValueOnce(makeAcceptedResponse("job-1"))
        .mockResolvedValueOnce(makeJobResponse("succeeded"));

      const { result } = renderHook(() => useAsyncAnswerPolling({ code: CODE }));

      await act(async () => {
        await result.current.submitPendingAnswerJob(makePending());
      });

      expect(pendingAnswerStoreWriteMock).toHaveBeenCalledWith(
        CODE,
        expect.objectContaining({ jobId: toJobId("job-1") }),
      );
    });

    it("clears pendingAnswerStore on successful poll result", async () => {
      apiMock
        .mockResolvedValueOnce(makeAcceptedResponse("job-1"))
        .mockResolvedValueOnce(makeJobResponse("succeeded"));

      const { result } = renderHook(() => useAsyncAnswerPolling({ code: CODE }));

      await act(async () => {
        await result.current.submitPendingAnswerJob(makePending());
      });

      expect(pendingAnswerStoreClearMock).toHaveBeenCalledWith(CODE);
    });

    it("throws coded error when initial POST fails", async () => {
      apiMock.mockResolvedValueOnce(makeErrorResponse(500, "Queue full", "QUEUE_FULL"));

      const { result } = renderHook(() => useAsyncAnswerPolling({ code: CODE }));

      await act(async () => {
        await expect(
          result.current.submitPendingAnswerJob(makePending()),
        ).rejects.toThrow("Queue full");
      });
    });
  });

  describe("pollAsyncAnswerJob", () => {
    it("continues polling on queued and running statuses", async () => {
      apiMock
        .mockResolvedValueOnce(makeJobResponse("queued"))
        .mockResolvedValueOnce(makeJobResponse("running"))
        .mockResolvedValueOnce(makeJobResponse("succeeded"));

      const { result } = renderHook(() => useAsyncAnswerPolling({ code: CODE }));

      await act(async () => {
        await result.current.pollAsyncAnswerJob("job-1");
      });

      expect(apiMock).toHaveBeenCalledTimes(3);
    });

    it("returns outcome immediately on succeeded status", async () => {
      apiMock.mockResolvedValueOnce(makeJobResponse("succeeded", { done: true }));

      const { result } = renderHook(() => useAsyncAnswerPolling({ code: CODE }));

      let outcome: unknown;
      await act(async () => {
        outcome = await result.current.pollAsyncAnswerJob("job-1");
      });

      expect(outcome).toMatchObject({ done: true });
      expect(apiMock).toHaveBeenCalledTimes(1);
    });

    it("throws coded error on canceled status with AI_RETRY_EXHAUSTED code", async () => {
      apiMock.mockResolvedValueOnce(makeJobResponse("canceled", {
        error_code: "AI_RETRY_EXHAUSTED",
        error_message: "Processing was canceled",
      }));

      const { result } = renderHook(() => useAsyncAnswerPolling({ code: CODE }));

      await act(async () => {
        await expect(
          result.current.pollAsyncAnswerJob("job-1"),
        ).rejects.toMatchObject({ code: "AI_RETRY_EXHAUSTED" });
      });
    });

    it("throws coded error on conflict status with TURN_CONFLICT code", async () => {
      apiMock.mockResolvedValueOnce(makeJobResponse("conflict", {
        error_code: "TURN_CONFLICT",
        error_message: "Turn is stale",
      }));

      const { result } = renderHook(() => useAsyncAnswerPolling({ code: CODE }));

      await act(async () => {
        await expect(
          result.current.pollAsyncAnswerJob("job-1"),
        ).rejects.toMatchObject({ code: "TURN_CONFLICT" });
      });
    });

    it("opens circuit breaker after consecutive 5xx failures", async () => {
      for (let i = 0; i < 5; i++) {
        apiMock.mockResolvedValueOnce(makeErrorResponse(500));
      }

      const { result } = renderHook(() => useAsyncAnswerPolling({ code: CODE }));

      await act(async () => {
        await expect(
          result.current.pollAsyncAnswerJob("job-1"),
        ).rejects.toMatchObject({ code: "ASYNC_POLL_CIRCUIT_OPEN" });
      });
    });

    it("opens circuit breaker after consecutive network errors", async () => {
      for (let i = 0; i < 5; i++) {
        apiMock.mockRejectedValueOnce(new ApiTimeoutError("Timeout"));
      }

      const { result } = renderHook(() => useAsyncAnswerPolling({ code: CODE }));

      await act(async () => {
        await expect(
          result.current.pollAsyncAnswerJob("job-1"),
        ).rejects.toMatchObject({ code: "ASYNC_POLL_CIRCUIT_OPEN" });
      });
    });

    it("throws polling canceled when isCanceled returns true", async () => {
      apiMock.mockResolvedValueOnce(makeJobResponse("queued"));

      const { result } = renderHook(() => useAsyncAnswerPolling({ code: CODE }));
      let callCount = 0;

      await act(async () => {
        await expect(
          result.current.pollAsyncAnswerJob("job-1", () => {
            callCount++;
            return callCount > 1;
          }),
        ).rejects.toThrow("Polling canceled");
      });
    });

    it("resets consecutive failure counter on successful response between errors", async () => {
      apiMock
        .mockResolvedValueOnce(makeErrorResponse(500))
        .mockResolvedValueOnce(makeErrorResponse(500))
        .mockResolvedValueOnce(makeJobResponse("queued"))
        .mockResolvedValueOnce(makeErrorResponse(500))
        .mockResolvedValueOnce(makeErrorResponse(500))
        .mockResolvedValueOnce(makeJobResponse("succeeded"));

      const { result } = renderHook(() => useAsyncAnswerPolling({ code: CODE }));

      let outcome: unknown;
      await act(async () => {
        outcome = await result.current.pollAsyncAnswerJob("job-1");
      });

      expect(outcome).toMatchObject({ done: false });
      expect(apiMock).toHaveBeenCalledTimes(6);
    });
  });
});
