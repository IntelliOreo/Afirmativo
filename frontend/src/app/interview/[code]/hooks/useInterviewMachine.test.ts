import { renderHook } from "@testing-library/react";
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AnswerOutcome } from "../models";
import type { PendingAnswerSubmission } from "../models";
import {
  toClientRequestId,
  toTurnId,
} from "../models";
import {
  interviewReducer,
  useInterviewMachine,
  type InterviewState,
} from "./useInterviewMachine";

const apiMock = vi.fn();
const submitPendingAnswerJobMock = vi.fn();
const submitPendingAnswerJobStable = (...args: unknown[]) => submitPendingAnswerJobMock(...args);

vi.mock("@/lib/api", () => ({
  api: (...args: unknown[]) => apiMock(...args),
}));

vi.mock("./useAsyncAnswerPolling", () => ({
  useAsyncAnswerPolling: () => ({
    submitPendingAnswerJob: submitPendingAnswerJobStable,
  }),
}));

const pendingAnswerStoreReadMock = vi.fn();
const pendingAnswerStoreWriteMock = vi.fn();
const pendingAnswerStoreClearMock = vi.fn();
const answerDraftStoreClearMock = vi.fn();
let pendingAnswerSnapshot: PendingAnswerSubmission | null = null;

vi.mock("@/lib/storage/pendingAnswerStore", () => ({
  read: (...args: unknown[]) => pendingAnswerStoreReadMock(...args),
  write: (...args: unknown[]) => pendingAnswerStoreWriteMock(...args),
  clear: (...args: unknown[]) => pendingAnswerStoreClearMock(...args),
}));

vi.mock("@/lib/storage/answerDraftStore", () => ({
  clear: (...args: unknown[]) => answerDraftStoreClearMock(...args),
}));

const question = {
  textEs: "Pregunta",
  textEn: "Question",
  area: "protected_ground",
  kind: "criterion" as const,
  turnId: toTurnId("turn-1"),
  questionNumber: 2,
  totalQuestions: 10,
};

function makeActiveState(): InterviewState {
  return {
    phase: "active",
    question,
    secondsLeft: 10,
    answerSecondsLeft: 10,
    textAnswer: "",
    inputMode: "text",
  };
}

function makePendingJob(answerText = "Answer"): PendingAnswerSubmission {
  return {
    clientRequestId: toClientRequestId("client-1"),
    turnId: question.turnId,
    answerText,
    questionText: question.textEn,
    createdAt: 123,
  };
}

describe("interviewReducer", () => {
  it("returns guard on unauthorized starts", () => {
    expect(
      interviewReducer({ phase: "loading" }, { type: "START_UNAUTHORIZED" }),
    ).toEqual({ phase: "guard" });
  });

  it("returns already completed done state", () => {
    expect(
      interviewReducer({ phase: "loading" }, { type: "START_COMPLETED" }),
    ).toEqual({ phase: "done", completionSource: "already_completed" });
  });

  it("builds the active state from a start success", () => {
    const next = interviewReducer(
      { phase: "loading" },
      { type: "START_SUCCEEDED", payload: { question, secondsLeft: 600, answerSecondsLeft: 240 } },
    );

    expect(next).toEqual({
      phase: "active",
      question,
      secondsLeft: 600,
      answerSecondsLeft: 240,
      textAnswer: "",
      inputMode: "text",
    });
  });

  it("updates text in the active state", () => {
    const next = interviewReducer(
      makeActiveState(),
      { type: "TEXT_CHANGED", payload: { value: "Updated" } },
    );

    expect(next).toMatchObject({ phase: "active", textAnswer: "Updated" });
  });

  it("moves into submitting with a canonical pending job", () => {
    const pendingJob = makePendingJob();
    const next = interviewReducer(
      makeActiveState(),
      { type: "SUBMIT_REQUESTED", payload: { pendingJob, submitMode: "question" } },
    );

    expect(next).toMatchObject({
      phase: "submitting",
      question,
      submitMode: "question",
      pendingJob,
    });
  });

  it("returns to active when a submit succeeds with another question", () => {
    const nextQuestion = { ...question, turnId: toTurnId("turn-2"), questionNumber: 3 };
    const next = interviewReducer(
      {
        phase: "submitting",
        question,
        secondsLeft: 0,
        answerSecondsLeft: 0,
        submitMode: "question",
        pendingJob: makePendingJob(),
        requestKind: "submit",
      },
      {
        type: "SUBMIT_SUCCEEDED",
        payload: {
          result: {
            done: false,
            nextQuestion,
            timerRemainingS: 420,
            answerSubmitWindowRemainingS: 240,
          },
        },
      },
    );

    expect(next).toEqual({
      phase: "active",
      question: nextQuestion,
      secondsLeft: 420,
      answerSecondsLeft: 240,
      textAnswer: "",
      inputMode: "text",
    });
  });

  it("moves to error on submit failure", () => {
    const next = interviewReducer(
      {
        phase: "submitting",
        question,
        secondsLeft: 0,
        answerSecondsLeft: 0,
        submitMode: "question",
        pendingJob: makePendingJob(),
        requestKind: "submit",
      },
      {
        type: "SUBMIT_FAILED",
        payload: { message: "Failed to submit", code: "TURN_CONFLICT" },
      },
    );

    expect(next).toEqual({
      phase: "error",
      message: "Failed to submit",
      code: "TURN_CONFLICT",
    });
  });

  it("keeps active state at zero after the tick boundary", () => {
    const next = interviewReducer(
      { ...makeActiveState(), secondsLeft: 1 },
      { type: "TICK" },
    );

    expect(next).toMatchObject({ phase: "active", secondsLeft: 0 });
  });
});

describe("useInterviewMachine", () => {
  beforeEach(() => {
    apiMock.mockReset();
    submitPendingAnswerJobMock.mockReset();
    pendingAnswerStoreReadMock.mockReset();
    pendingAnswerStoreWriteMock.mockReset();
    pendingAnswerStoreClearMock.mockReset();
    answerDraftStoreClearMock.mockReset();
    pendingAnswerSnapshot = null;
    pendingAnswerStoreReadMock.mockImplementation(() => pendingAnswerSnapshot);
    pendingAnswerStoreWriteMock.mockImplementation((_code, pendingJob) => {
      pendingAnswerSnapshot = pendingJob as PendingAnswerSubmission;
    });
    pendingAnswerStoreClearMock.mockImplementation(() => {
      pendingAnswerSnapshot = null;
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("keeps the machine active at zero until forced finalization is orchestrated elsewhere", async () => {
    vi.useFakeTimers();
    apiMock.mockResolvedValue({
      ok: true,
      status: 200,
      data: {
        question: {
          text_es: "Pregunta",
          text_en: "Question",
          area: "protected_ground",
          kind: "criterion",
          turn_id: "turn-1",
          question_number: 2,
          total_questions: 10,
        },
        timer_remaining_s: 1,
        answer_submit_window_remaining_s: 1,
        language: "en",
      },
    });

    const setLang = vi.fn();
    const { result } = renderHook(() => useInterviewMachine({
      code: "AP-123",
      lang: "en",
      langInitialized: true,
      setLang,
    }));

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(result.current.state.phase).toBe("active");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(submitPendingAnswerJobMock).not.toHaveBeenCalled();
    expect(result.current.state).toMatchObject({
      phase: "active",
      secondsLeft: 0,
      answerSecondsLeft: 0,
    });
  });

  it("recovers a persisted pending answer before calling start", async () => {
    pendingAnswerStoreReadMock.mockReturnValue(makePendingJob());
    submitPendingAnswerJobMock.mockResolvedValue({
      done: false,
      nextQuestion: {
        ...question,
        turnId: toTurnId("turn-2"),
        questionNumber: 3,
      },
      timerRemainingS: 420,
      answerSubmitWindowRemainingS: 240,
    } satisfies AnswerOutcome);

    const setLang = vi.fn();
    const { result } = renderHook(() => useInterviewMachine({
      code: "AP-123",
      lang: "en",
      langInitialized: true,
      setLang,
    }));

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(submitPendingAnswerJobMock).toHaveBeenCalledTimes(1);
    expect(apiMock).not.toHaveBeenCalled();
    expect(answerDraftStoreClearMock).toHaveBeenCalledWith("AP-123", question.turnId);
    expect(result.current.state).toMatchObject({
      phase: "active",
      question: {
        turnId: "turn-2",
        questionNumber: 3,
      },
    });
  });

  it("keeps the in-flight submit alive while the submitting timer ticks", async () => {
    vi.useFakeTimers();
    apiMock.mockResolvedValue({
      ok: true,
      status: 200,
      data: {
        question: {
          text_es: "Pregunta",
          text_en: "Question",
          area: "protected_ground",
          kind: "criterion",
          turn_id: "turn-1",
          question_number: 2,
          total_questions: 10,
        },
        timer_remaining_s: 5,
        answer_submit_window_remaining_s: 240,
        language: "en",
      },
    });
    submitPendingAnswerJobMock.mockImplementation(async () => {
      await new Promise<void>((resolve) => {
        setTimeout(resolve, 1100);
      });
      return {
        done: false,
        nextQuestion: {
          ...question,
          turnId: toTurnId("turn-2"),
          questionNumber: 3,
        },
        timerRemainingS: 420,
        answerSubmitWindowRemainingS: 240,
      } satisfies AnswerOutcome;
    });

    const setLang = vi.fn();
    const { result } = renderHook(() => useInterviewMachine({
      code: "AP-123",
      lang: "en",
      langInitialized: true,
      setLang,
    }));

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(result.current.state.phase).toBe("active");

    act(() => {
      result.current.requestSubmit("My answer");
    });

    expect(result.current.state.phase).toBe("submitting");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1100);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(submitPendingAnswerJobMock).toHaveBeenCalledTimes(1);
    expect(result.current.state.phase).toBe("active");

    expect(result.current.state).toMatchObject({
      phase: "active",
      secondsLeft: 420,
      question: {
        turnId: "turn-2",
        questionNumber: 3,
      },
    });
  });

  it("surfaces submit failures without faking completion", async () => {
    vi.useFakeTimers();
    apiMock.mockResolvedValue({
      ok: true,
      status: 200,
      data: {
        question: {
          text_es: "Pregunta",
          text_en: "Question",
          area: "protected_ground",
          kind: "criterion",
          turn_id: "turn-1",
          question_number: 2,
          total_questions: 10,
        },
        timer_remaining_s: 5,
        answer_submit_window_remaining_s: 240,
        language: "en",
      },
    });
    submitPendingAnswerJobMock.mockRejectedValue(new Error("backend uncertain"));

    const setLang = vi.fn();
    const { result } = renderHook(() => useInterviewMachine({
      code: "AP-123",
      lang: "en",
      langInitialized: true,
      setLang,
    }));

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    act(() => {
      result.current.requestSubmit("Last answer");
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(pendingAnswerStoreClearMock).not.toHaveBeenCalled();
    expect(result.current.canRetryPendingRecovery).toBe(true);
    expect(result.current.state).toEqual({
      phase: "error",
      message: "backend uncertain",
      code: "",
    });
  });
});
