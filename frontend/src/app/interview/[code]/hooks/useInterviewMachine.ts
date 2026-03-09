"use client";

import { useCallback, useEffect, useReducer, useRef } from "react";
import type { Dispatch, SetStateAction } from "react";
import { api } from "@/lib/api";
import * as pendingAnswerStore from "@/lib/storage/pendingAnswerStore";
import type { Lang } from "@/lib/language";
import type { StartResponseDto } from "../dto";
import { mapStartResponse } from "../mappers";
import type {
  AnswerOutcome,
  PendingAnswerSubmission,
  Question,
} from "../models";
import {
  getQuestionText,
  toClientRequestId,
} from "../models";
import type { CompletionSource, InputMode, SubmitMode } from "../viewTypes";
import { useAsyncAnswerPolling } from "./useAsyncAnswerPolling";
import {
  extractErrorCode,
  isCompletedResponse,
  isUnauthorizedResponse,
  makeClientRequestId,
  shouldClearPendingAnswerOnError,
} from "../utils";

type RequestKind = "submit" | "recovery";

export type InterviewState =
  | { phase: "guard" }
  | { phase: "loading" }
  | {
    phase: "active";
    question: Question;
    secondsLeft: number;
    textAnswer: string;
    inputMode: InputMode;
  }
  | {
    phase: "submitting";
    question: Question;
    secondsLeft: number;
    submitMode: SubmitMode;
    pendingJob: PendingAnswerSubmission;
    requestKind: RequestKind;
  }
  | { phase: "done"; completionSource: CompletionSource }
  | { phase: "error"; message: string; code?: string };

export type InterviewAction =
  | { type: "START_REQUESTED" }
  | { type: "START_UNAUTHORIZED" }
  | { type: "START_COMPLETED" }
  | { type: "START_SUCCEEDED"; payload: { question: Question; secondsLeft: number } }
  | { type: "START_FAILED"; payload: { message: string; code?: string } }
  | { type: "TEXT_CHANGED"; payload: { value: string } }
  | { type: "INPUT_MODE_CHANGED"; payload: { mode: InputMode } }
  | { type: "TICK" }
  | { type: "AUTO_SUBMIT_REQUESTED"; payload: { pendingJob: PendingAnswerSubmission } }
  | { type: "SUBMIT_REQUESTED"; payload: { pendingJob: PendingAnswerSubmission } }
  | { type: "SUBMIT_SUCCEEDED"; payload: { result: AnswerOutcome } }
  | { type: "SUBMIT_FAILED"; payload: { message: string; code?: string } }
  | { type: "RECOVERY_REQUESTED"; payload: { pendingJob: PendingAnswerSubmission } }
  | { type: "RECOVERY_SUCCEEDED"; payload: { result: AnswerOutcome } }
  | { type: "RECOVERY_FAILED"; payload: { message: string; code?: string } };

interface UseInterviewMachineParams {
  code: string;
  lang: Lang;
  langInitialized: boolean;
  setLang: Dispatch<SetStateAction<Lang>>;
}

interface UseInterviewMachineResult {
  state: InterviewState;
  dispatch: Dispatch<InterviewAction>;
  requestSubmit: (answerText: string, submitMode?: SubmitMode) => void;
}

function toActiveState(
  question: Question,
  secondsLeft: number,
): Extract<InterviewState, { phase: "active" }> {
  return {
    phase: "active",
    question,
    secondsLeft,
    textAnswer: "",
    inputMode: "text",
  };
}

function applyAnswerOutcome(
  currentQuestion: Question,
  outcome: AnswerOutcome,
): InterviewState {
  if (outcome.done) {
    return {
      phase: "done",
      completionSource: "finished",
    };
  }

  return toActiveState(outcome.nextQuestion ?? currentQuestion, outcome.timerRemainingS);
}

export function interviewReducer(state: InterviewState, action: InterviewAction): InterviewState {
  switch (action.type) {
    case "START_REQUESTED":
      return { phase: "loading" };
    case "START_UNAUTHORIZED":
      return { phase: "guard" };
    case "START_COMPLETED":
      return { phase: "done", completionSource: "already_completed" };
    case "START_SUCCEEDED":
      return toActiveState(action.payload.question, action.payload.secondsLeft);
    case "START_FAILED":
      return { phase: "error", message: action.payload.message, code: action.payload.code };
    case "TEXT_CHANGED":
      if (state.phase !== "active") return state;
      return { ...state, textAnswer: action.payload.value };
    case "INPUT_MODE_CHANGED":
      if (state.phase !== "active") return state;
      return { ...state, inputMode: action.payload.mode };
    case "TICK":
      if (state.phase === "active" || state.phase === "submitting") {
        return {
          ...state,
          secondsLeft: Math.max(0, state.secondsLeft - 1),
        };
      }
      return state;
    case "AUTO_SUBMIT_REQUESTED":
      if (state.phase !== "active") return state;
      return {
        phase: "submitting",
        question: state.question,
        secondsLeft: state.secondsLeft,
        submitMode: "finalAuto",
        pendingJob: action.payload.pendingJob,
        requestKind: "submit",
      };
    case "SUBMIT_REQUESTED":
      if (state.phase !== "active") return state;
      return {
        phase: "submitting",
        question: state.question,
        secondsLeft: state.secondsLeft,
        submitMode: "question",
        pendingJob: action.payload.pendingJob,
        requestKind: "submit",
      };
    case "RECOVERY_REQUESTED":
      if (state.phase !== "active") return state;
      return {
        phase: "submitting",
        question: state.question,
        secondsLeft: state.secondsLeft,
        submitMode: "question",
        pendingJob: action.payload.pendingJob,
        requestKind: "recovery",
      };
    case "SUBMIT_SUCCEEDED":
    case "RECOVERY_SUCCEEDED":
      if (state.phase !== "submitting") return state;
      return applyAnswerOutcome(state.question, action.payload.result);
    case "SUBMIT_FAILED":
    case "RECOVERY_FAILED":
      return { phase: "error", message: action.payload.message, code: action.payload.code };
    default:
      return state;
  }
}

function buildPendingJob(question: Question, lang: Lang, answerText: string): PendingAnswerSubmission {
  return {
    clientRequestId: toClientRequestId(makeClientRequestId()),
    turnId: question.turnId,
    answerText,
    questionText: getQuestionText(question, lang),
    createdAt: Date.now(),
  };
}

export function useInterviewMachine({
  code,
  lang,
  langInitialized,
  setLang,
}: UseInterviewMachineParams): UseInterviewMachineResult {
  const [state, dispatch] = useReducer(interviewReducer, { phase: "guard" });
  const startRequestKeyRef = useRef<string>("");
  const activeSubmissionKeyRef = useRef<string>("");
  const { submitPendingAnswerJob } = useAsyncAnswerPolling({ code });
  const submissionPendingJob = state.phase === "submitting" ? state.pendingJob : null;
  const submissionRequestKind = state.phase === "submitting" ? state.requestKind : null;
  const submissionMode = state.phase === "submitting" ? state.submitMode : null;

  const requestSubmit = useCallback((answerText: string, submitMode: SubmitMode = "question") => {
    if (state.phase !== "active") return;

    const pendingJob = buildPendingJob(state.question, lang, answerText.trim());
    dispatch({
      type: submitMode === "finalAuto" ? "AUTO_SUBMIT_REQUESTED" : "SUBMIT_REQUESTED",
      payload: { pendingJob },
    });
  }, [lang, state]);

  useEffect(() => {
    if (!langInitialized) return;

    const requestKey = `${code}:start`;
    if (startRequestKeyRef.current === requestKey) return;
    startRequestKeyRef.current = requestKey;

    let canceled = false;
    dispatch({ type: "START_REQUESTED" });

    void (async () => {
      try {
        const { ok, status, data } = await api<StartResponseDto>("/api/interview/start", {
          method: "POST",
          body: { session_code: code, language: lang },
          credentials: "include",
        });

        if (canceled) return;

        if (!ok || !data) {
          const errorMessage = data?.error ?? "Failed to start";
          if (isUnauthorizedResponse(status, data?.code)) {
            dispatch({ type: "START_UNAUTHORIZED" });
            return;
          }
          if (isCompletedResponse(status, data?.code, errorMessage)) {
            dispatch({ type: "START_COMPLETED" });
            return;
          }
          dispatch({
            type: "START_FAILED",
            payload: { message: errorMessage, code: data?.code },
          });
          return;
        }

        const mapped = mapStartResponse(data);
        dispatch({
          type: "START_SUCCEEDED",
          payload: { question: mapped.question, secondsLeft: mapped.timerRemainingS },
        });

        if (mapped.language === "en" || mapped.language === "es") {
          setLang(mapped.language);
        }

        const pendingJob = pendingAnswerStore.read(code);
        if (pendingJob) {
          dispatch({ type: "RECOVERY_REQUESTED", payload: { pendingJob } });
        }
      } catch (err) {
        if (canceled) return;
        dispatch({
          type: "START_FAILED",
          payload: {
            message: err instanceof Error ? err.message : "Unknown error",
            code: extractErrorCode(err),
          },
        });
      }
    })();

    return () => {
      canceled = true;
    };
  }, [code, lang, langInitialized, setLang]);

  useEffect(() => {
    if (state.phase !== "active" || state.secondsLeft !== 0) return;
    dispatch({
      type: "AUTO_SUBMIT_REQUESTED",
      payload: { pendingJob: buildPendingJob(state.question, lang, state.textAnswer.trim()) },
    });
  }, [lang, state]);

  useEffect(() => {
    if (state.phase !== "active" && state.phase !== "submitting") return;
    if (state.secondsLeft <= 0) return;

    const intervalId = window.setInterval(() => {
      dispatch({ type: "TICK" });
    }, 1000);

    return () => {
      window.clearInterval(intervalId);
    };
  }, [state]);

  useEffect(() => {
    if (!submissionPendingJob || !submissionRequestKind || !submissionMode) {
      activeSubmissionKeyRef.current = "";
      return;
    }

    const submissionKey = `${submissionRequestKind}:${submissionPendingJob.clientRequestId}`;
    if (activeSubmissionKeyRef.current === submissionKey) return;
    activeSubmissionKeyRef.current = submissionKey;

    let canceled = false;
    pendingAnswerStore.write(code, submissionPendingJob);

    void (async () => {
      try {
        const result = await submitPendingAnswerJob(submissionPendingJob, () => canceled);
        if (canceled) return;

        dispatch({
          type: submissionRequestKind === "recovery" ? "RECOVERY_SUCCEEDED" : "SUBMIT_SUCCEEDED",
          payload: { result },
        });
      } catch (err) {
        if (canceled) return;

        if (submissionMode === "finalAuto") {
          pendingAnswerStore.clear(code);
          dispatch({
            type: "SUBMIT_SUCCEEDED",
            payload: { result: { done: true, timerRemainingS: 0 } },
          });
          return;
        }

        const errorCode = extractErrorCode(err);
        if (shouldClearPendingAnswerOnError(errorCode)) {
          pendingAnswerStore.clear(code);
        }

        dispatch({
          type: submissionRequestKind === "recovery" ? "RECOVERY_FAILED" : "SUBMIT_FAILED",
          payload: {
            message: err instanceof Error ? err.message : "Unknown error",
            code: errorCode,
          },
        });
      }
    })();

    return () => {
      canceled = true;
    };
  }, [code, submissionMode, submissionPendingJob, submissionRequestKind, submitPendingAnswerJob]);

  return { state, dispatch, requestSubmit };
}
