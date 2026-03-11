"use client";

import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import { api } from "@/lib/api";
import * as answerDraftStore from "@/lib/storage/answerDraftStore";
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
  isPendingRecoveryRetryableErrorCode,
  isUnauthorizedResponse,
  makeClientRequestId,
  shouldAttemptStartAfterRecoveryError,
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
      answerSecondsLeft: number;
      textAnswer: string;
      inputMode: InputMode;
    }
  | {
      phase: "submitting";
      question: Question;
      secondsLeft: number;
      answerSecondsLeft: number;
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
  | { type: "BOOT_RECOVERY_COMPLETED" }
  | { type: "START_SUCCEEDED"; payload: { question: Question; secondsLeft: number; answerSecondsLeft: number } }
  | { type: "START_FAILED"; payload: { message: string; code?: string } }
  | { type: "TEXT_CHANGED"; payload: { value: string } }
  | { type: "INPUT_MODE_CHANGED"; payload: { mode: InputMode } }
  | { type: "TICK" }
  | { type: "SUBMIT_REQUESTED"; payload: { pendingJob: PendingAnswerSubmission; submitMode: SubmitMode } }
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
  retryPendingRecovery: () => void;
  canRetryPendingRecovery: boolean;
}

function toActiveState(
  question: Question,
  secondsLeft: number,
  answerSecondsLeft: number,
): Extract<InterviewState, { phase: "active" }> {
  return {
    phase: "active",
    question,
    secondsLeft,
    answerSecondsLeft,
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

  return toActiveState(
    outcome.nextQuestion ?? currentQuestion,
    outcome.timerRemainingS,
    outcome.answerSubmitWindowRemainingS,
  );
}

export function interviewReducer(state: InterviewState, action: InterviewAction): InterviewState {
  switch (action.type) {
    case "START_REQUESTED":
      return { phase: "loading" };
    case "START_UNAUTHORIZED":
      return { phase: "guard" };
    case "START_COMPLETED":
      return { phase: "done", completionSource: "already_completed" };
    case "BOOT_RECOVERY_COMPLETED":
      return { phase: "done", completionSource: "finished" };
    case "START_SUCCEEDED":
      return toActiveState(
        action.payload.question,
        action.payload.secondsLeft,
        action.payload.answerSecondsLeft,
      );
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
          answerSecondsLeft: Math.max(0, state.answerSecondsLeft - 1),
        };
      }
      return state;
    case "SUBMIT_REQUESTED":
      if (state.phase !== "active") return state;
      return {
        phase: "submitting",
        question: state.question,
        secondsLeft: state.secondsLeft,
        answerSecondsLeft: state.answerSecondsLeft,
        submitMode: action.payload.submitMode,
        pendingJob: action.payload.pendingJob,
        requestKind: "submit",
      };
    case "RECOVERY_REQUESTED":
      if (state.phase !== "active") return state;
      return {
        phase: "submitting",
        question: state.question,
        secondsLeft: state.secondsLeft,
        answerSecondsLeft: state.answerSecondsLeft,
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

function clearPendingSubmission(sessionCode: string): void {
  pendingAnswerStore.clear(sessionCode);
}

function clearPendingSubmissionAndDraft(sessionCode: string, pendingJob: PendingAnswerSubmission): void {
  pendingAnswerStore.clear(sessionCode);
  answerDraftStore.clear(sessionCode, pendingJob.turnId);
}

export function useInterviewMachine({
  code,
  lang,
  langInitialized,
  setLang,
}: UseInterviewMachineParams): UseInterviewMachineResult {
  const [state, dispatch] = useReducer(interviewReducer, { phase: "guard" });
  const [bootAttempt, setBootAttempt] = useState(0);
  const startRequestKeyRef = useRef<string>("");
  const activeSubmissionKeyRef = useRef<string>("");
  const langRef = useRef<Lang>(lang);
  const { submitPendingAnswerJob } = useAsyncAnswerPolling({ code });
  const submissionPendingJob = state.phase === "submitting" ? state.pendingJob : null;
  const submissionRequestKind = state.phase === "submitting" ? state.requestKind : null;
  const submissionMode = state.phase === "submitting" ? state.submitMode : null;

  useEffect(() => {
    langRef.current = lang;
  }, [lang]);

  const requestSubmit = useCallback((answerText: string, submitMode: SubmitMode = "question") => {
    if (state.phase !== "active") return;

    const pendingJob = buildPendingJob(state.question, lang, answerText.trim());
    dispatch({
      type: "SUBMIT_REQUESTED",
      payload: { pendingJob, submitMode },
    });
  }, [lang, state]);

  useEffect(() => {
    if (!langInitialized) return;

    const requestKey = `${code}:start:${bootAttempt}`;
    if (startRequestKeyRef.current === requestKey) return;
    startRequestKeyRef.current = requestKey;

    let canceled = false;
    dispatch({ type: "START_REQUESTED" });

    void (async () => {
      const pendingJobBeforeStart = pendingAnswerStore.read(code);
      let recoveryTerminalCode = "";

      try {
        if (pendingJobBeforeStart) {
          try {
            const recoveryResult = await submitPendingAnswerJob(pendingJobBeforeStart, () => canceled);
            if (canceled) return;

            answerDraftStore.clear(code, pendingJobBeforeStart.turnId);
            if (recoveryResult.done) {
              dispatch({ type: "BOOT_RECOVERY_COMPLETED" });
              return;
            }
            if (!recoveryResult.nextQuestion) {
              dispatch({
                type: "START_FAILED",
                payload: {
                  message: "Recovered answer completed without a next question",
                  code: "RECOVERY_INVALID",
                },
              });
              return;
            }

            dispatch({
              type: "START_SUCCEEDED",
              payload: {
                question: recoveryResult.nextQuestion,
                secondsLeft: recoveryResult.timerRemainingS,
                answerSecondsLeft: recoveryResult.answerSubmitWindowRemainingS,
              },
            });
            return;
          } catch (err) {
            if (canceled) return;

            recoveryTerminalCode = extractErrorCode(err);
            if (shouldClearPendingAnswerOnError(recoveryTerminalCode)) {
              clearPendingSubmission(code);
            }
            if (!shouldAttemptStartAfterRecoveryError(recoveryTerminalCode)) {
              dispatch({
                type: "START_FAILED",
                payload: {
                  message: err instanceof Error ? err.message : "Unknown error",
                  code: recoveryTerminalCode,
                },
              });
              return;
            }
          }
        }

        const { ok, status, data } = await api<StartResponseDto>("/api/interview/start", {
          method: "POST",
          body: { session_code: code, language: lang },
          credentials: "include",
        });

        if (canceled) return;

        if (!ok || !data) {
          const errorMessage = data?.error ?? "Failed to start";
          if (isUnauthorizedResponse(status, data?.code)) {
            if (pendingJobBeforeStart) {
              clearPendingSubmission(code);
            }
            dispatch({ type: "START_UNAUTHORIZED" });
            return;
          }
          if (isCompletedResponse(status, data?.code, errorMessage)) {
            if (pendingJobBeforeStart) {
              clearPendingSubmissionAndDraft(code, pendingJobBeforeStart);
            }
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
        if (pendingJobBeforeStart) {
          if (mapped.question.turnId !== pendingJobBeforeStart.turnId) {
            clearPendingSubmissionAndDraft(code, pendingJobBeforeStart);
          } else if (recoveryTerminalCode !== "") {
            clearPendingSubmission(code);
          }
        }

        dispatch({
          type: "START_SUCCEEDED",
          payload: {
            question: mapped.question,
            secondsLeft: mapped.timerRemainingS,
            answerSecondsLeft: mapped.answerSubmitWindowRemainingS,
          },
        });

        if (mapped.language === "en" || mapped.language === "es") {
          setLang(mapped.language);
        }
      } catch (err) {
        if (canceled) return;
        const errorCode = extractErrorCode(err);
        if (pendingJobBeforeStart && shouldClearPendingAnswerOnError(errorCode)) {
          clearPendingSubmission(code);
        }
        dispatch({
          type: "START_FAILED",
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
  }, [bootAttempt, code, lang, langInitialized, setLang, submitPendingAnswerJob]);

  useEffect(() => {
    const isTickingPhase = state.phase === "active" || state.phase === "submitting";
    if (!isTickingPhase) return;
    if (state.secondsLeft <= 0) return;

    const intervalId = window.setInterval(() => {
      dispatch({ type: "TICK" });
    }, 1000);

    return () => {
      window.clearInterval(intervalId);
    };
  }, [
    dispatch,
    state.phase,
    state.phase === "active" || state.phase === "submitting"
      ? state.secondsLeft
      : 0,
  ]);

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

        answerDraftStore.clear(code, submissionPendingJob.turnId);

        dispatch({
          type: submissionRequestKind === "recovery" ? "RECOVERY_SUCCEEDED" : "SUBMIT_SUCCEEDED",
          payload: { result },
        });
      } catch (err) {
        if (canceled) return;

        if (submissionMode === "finalAuto") {
          dispatch({
            type: "SUBMIT_FAILED",
            payload: {
              message: langRef.current === "es"
                ? "No se pudo confirmar el envío final automático. Recargue para continuar."
                : "Automatic final submission could not be confirmed. Reload to continue.",
              code: "FINAL_AUTO_RECOVERY_REQUIRED",
            },
          });
          return;
        }

        const errorCode = extractErrorCode(err);
        if (shouldClearPendingAnswerOnError(errorCode)) {
          clearPendingSubmission(code);
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

  const retryPendingRecovery = useCallback(() => {
    if (!pendingAnswerStore.read(code)) return;
    startRequestKeyRef.current = "";
    setBootAttempt((current) => current + 1);
  }, [code]);

  const canRetryPendingRecovery =
    state.phase === "error"
    && pendingAnswerStore.read(code) != null
    && isPendingRecoveryRetryableErrorCode(state.code ?? "");

  return { state, dispatch, requestSubmit, retryPendingRecovery, canRetryPendingRecovery };
}
