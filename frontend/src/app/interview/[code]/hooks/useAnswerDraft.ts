"use client";

import { useEffect, useRef } from "react";
import type { Lang } from "@/lib/language";
import * as answerDraftStore from "@/lib/storage/answerDraftStore";
import type { Question } from "../models";
import { getQuestionText } from "../models";
import type { InputMode, VoiceRecorderState } from "../viewTypes";

interface UseAnswerDraftParams {
  code: string;
  currentQuestion: Question;
  textAnswer: string;
  inputMode: InputMode;
  voiceRecorderState: VoiceRecorderState;
  lang: Lang;
  phase: "active" | "submitting";
  onTextChange: (value: string) => void;
  onInputModeChange: (mode: InputMode) => void;
}

export function useAnswerDraft({
  code,
  currentQuestion,
  textAnswer,
  inputMode,
  voiceRecorderState,
  lang,
  phase,
  onTextChange,
  onInputModeChange,
}: UseAnswerDraftParams): void {
  const restoredDraftTurnRef = useRef("");

  // Reset restored draft tracking on question change
  useEffect(() => {
    restoredDraftTurnRef.current = "";
  }, [currentQuestion.turnId]);

  // Restore saved draft on question mount
  useEffect(() => {
    if (phase !== "active") return;

    answerDraftStore.clearStale(code, currentQuestion.turnId);
    if (restoredDraftTurnRef.current === currentQuestion.turnId) return;
    restoredDraftTurnRef.current = currentQuestion.turnId;

    const savedDraft = answerDraftStore.read(code, currentQuestion.turnId);
    if (!savedDraft) return;

    onTextChange(savedDraft.draftText);
    if (savedDraft.source === "voice_review") {
      onInputModeChange("voice");
    }
  }, [code, currentQuestion.turnId, onTextChange, onInputModeChange, phase]);

  // Persist draft to localStorage on change
  useEffect(() => {
    if (phase !== "active") return;

    if (!textAnswer.trim()) {
      answerDraftStore.clear(code, currentQuestion.turnId);
      return;
    }

    answerDraftStore.write(code, {
      turnId: currentQuestion.turnId,
      questionText: getQuestionText(currentQuestion, lang),
      draftText: textAnswer,
      source: inputMode === "voice" && voiceRecorderState === "review_ready"
        ? "voice_review"
        : "text",
      updatedAt: Date.now(),
    });
  }, [code, currentQuestion, inputMode, lang, phase, textAnswer, voiceRecorderState]);
}
