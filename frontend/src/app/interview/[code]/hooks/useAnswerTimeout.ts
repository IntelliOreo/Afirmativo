"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getTimeoutDialogState } from "../lib/getTimeoutDialogState";
import type { Question } from "../models";
import type { InputMode, VoiceRecorderState } from "../viewTypes";

interface UseAnswerTimeoutParams {
  phase: "active" | "submitting";
  currentQuestion: Question;
  isTimerExpired: boolean;
  answerSecondsLeft: number;
  inputMode: InputMode;
  voiceRecorderState: VoiceRecorderState;
  textAnswer: string;
  completeVoiceRecording: () => void;
  onAutoReviewVoice: () => void;
  requestSubmit: (answerText: string) => void;
}

interface UseAnswerTimeoutResult {
  shouldShowTimeoutDialog: boolean;
  isTimeoutDialogTranscribing: boolean;
  isTimeoutDialogInterrupted: boolean;
  handleTimeoutSubmit: () => void;
}

export function useAnswerTimeout({
  phase,
  currentQuestion,
  isTimerExpired,
  answerSecondsLeft,
  inputMode,
  voiceRecorderState,
  textAnswer,
  completeVoiceRecording,
  onAutoReviewVoice,
  requestSubmit,
}: UseAnswerTimeoutParams): UseAnswerTimeoutResult {
  const [showTimeoutDialog, setShowTimeoutDialog] = useState(false);
  const timedOutTurnRef = useRef("");
  const timeoutReviewTurnRef = useRef("");
  const liveExpiredTurnRef = useRef("");
  const previousAnswerWindowRef = useRef<{ turnId: string; answerSecondsLeft: number } | null>(null);

  // Reset all timeout state when question changes
  useEffect(() => {
    setShowTimeoutDialog(false);
    timedOutTurnRef.current = "";
    timeoutReviewTurnRef.current = "";
    liveExpiredTurnRef.current = "";
    previousAnswerWindowRef.current = null;
  }, [currentQuestion.turnId]);

  // Track answer window to detect live expiration (>0 → <=0 transition)
  useEffect(() => {
    if (phase !== "active" || currentQuestion.kind !== "criterion") {
      previousAnswerWindowRef.current = null;
      return;
    }

    const previous = previousAnswerWindowRef.current;
    if (
      previous
      && previous.turnId === currentQuestion.turnId
      && previous.answerSecondsLeft > 0
      && answerSecondsLeft <= 0
    ) {
      liveExpiredTurnRef.current = currentQuestion.turnId;
    }

    previousAnswerWindowRef.current = {
      turnId: currentQuestion.turnId,
      answerSecondsLeft,
    };
  }, [answerSecondsLeft, currentQuestion, phase]);

  // Auto-complete voice recording when timer expires
  useEffect(() => {
    if (!isTimerExpired) return;
    if (voiceRecorderState === "recording" || voiceRecorderState === "paused") {
      completeVoiceRecording();
    }
  }, [completeVoiceRecording, isTimerExpired, voiceRecorderState]);

  // Trigger timeout dialog for criterion questions
  useEffect(() => {
    if (phase !== "active" || currentQuestion.kind !== "criterion") {
      setShowTimeoutDialog(false);
      return;
    }
    if (!isTimerExpired) return;
    if (timedOutTurnRef.current === currentQuestion.turnId) return;
    timedOutTurnRef.current = currentQuestion.turnId;
    setShowTimeoutDialog(true);
  }, [currentQuestion, isTimerExpired, phase]);

  const {
    shouldShowTimeoutDialog,
    shouldAutoReviewTimedOutVoice,
    isTimeoutDialogTranscribing,
    isTimeoutDialogInterrupted,
  } = getTimeoutDialogState({
    showTimeoutDialog,
    phase,
    currentQuestion,
    isTimerExpired,
    inputMode,
    voiceRecorderState,
    textAnswer,
    liveExpiredTurnId: liveExpiredTurnRef.current,
    timeoutReviewTurnId: timeoutReviewTurnRef.current,
  });

  // Auto-review voice when timed out with audio ready
  useEffect(() => {
    if (!shouldAutoReviewTimedOutVoice) return;
    timeoutReviewTurnRef.current = currentQuestion.turnId;
    onAutoReviewVoice();
  }, [currentQuestion.turnId, onAutoReviewVoice, shouldAutoReviewTimedOutVoice]);

  const handleTimeoutSubmit = useCallback(() => {
    if (phase !== "active") return;
    requestSubmit(textAnswer);
  }, [phase, requestSubmit, textAnswer]);

  return {
    shouldShowTimeoutDialog,
    isTimeoutDialogTranscribing,
    isTimeoutDialogInterrupted,
    handleTimeoutSubmit,
  };
}
