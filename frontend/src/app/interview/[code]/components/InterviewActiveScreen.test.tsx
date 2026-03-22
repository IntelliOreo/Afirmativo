import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TurnId } from "../models";
import type { Question } from "../models";
import { InterviewActiveScreen } from "./InterviewActiveScreen";

const voiceRecorderMock = vi.fn();
const micDialogMock = vi.fn();
const answerDraftReadMock = vi.fn(() => null);
const answerDraftWriteMock = vi.fn();
const answerDraftClearMock = vi.fn();
const answerDraftClearStaleMock = vi.fn();

vi.mock("../hooks/useVoiceRecorder", () => ({
  useVoiceRecorder: (...args: unknown[]) => voiceRecorderMock(...args),
}));

vi.mock("../hooks/useMicrophoneDialogState", () => ({
  useMicrophoneDialogState: (...args: unknown[]) => micDialogMock(...args),
}));

vi.mock("@/lib/storage/answerDraftStore", () => ({
  read: (...args: unknown[]) => answerDraftReadMock(...args),
  write: (...args: unknown[]) => answerDraftWriteMock(...args),
  clear: (...args: unknown[]) => answerDraftClearMock(...args),
  clearStale: (...args: unknown[]) => answerDraftClearStaleMock(...args),
}));

function makeVoiceRecorderState(overrides: Record<string, unknown> = {}) {
  return {
    voiceRecorderState: "idle",
    micWarmState: "cold",
    voiceDurationSeconds: 0,
    voiceWarningSeconds: null,
    voiceBlob: null,
    voicePreviewUrl: null,
    isVoicePreviewPlaying: false,
    voiceError: null,
    voiceInfo: null,
    isRecordingActive: false,
    isRecordingPaused: false,
    prepareMicrophone: vi.fn(async () => false),
    startVoiceRecording: vi.fn(async () => {}),
    completeVoiceRecording: vi.fn(),
    discardVoiceRecording: vi.fn(),
    toggleVoicePreviewPlayback: vi.fn(async () => {}),
    reviewVoiceRecording: vi.fn(async () => null),
    setVoiceErrorFeedback: vi.fn(),
    ...overrides,
  };
}

function makeMicDialogState(overrides: Record<string, unknown> = {}) {
  return {
    showMicrophoneDialog: false,
    activeMicDialogMode: null,
    micDialogUiState: "idle",
    handleEnableMicrophone: vi.fn(),
    handleDismissMicrophonePrompt: vi.fn(),
    ...overrides,
  };
}

function makeQuestion(overrides: Partial<Question> = {}): Question {
  return {
    textEs: "Pregunta",
    textEn: "Question",
    area: "protected_ground",
    kind: "criterion",
    turnId: "turn-1" as TurnId,
    questionNumber: 2,
    totalQuestions: 25,
    ...overrides,
  };
}

interface Props {
  lang?: "en" | "es";
  code?: string;
  phase?: "active" | "submitting";
  currentQuestion?: Question;
  textAnswer?: string;
  inputMode?: "text" | "voice";
  answerSecondsLeft?: number;
  isTimerExpired?: boolean;
  secondsLeft?: number;
  hasMicOptIn?: boolean;
  onMicOptIn?: () => void;
  onTextChange?: (value: string) => void;
  onInputModeChange?: (mode: "text" | "voice") => void;
  requestSubmit?: (answerText: string) => void;
}

function renderScreen(props: Props = {}) {
  const defaultProps = {
    lang: "en" as const,
    code: "AP-TEST",
    phase: "active" as const,
    currentQuestion: makeQuestion(),
    textAnswer: "",
    inputMode: "text" as const,
    answerSecondsLeft: 120,
    isTimerExpired: false,
    secondsLeft: 600,
    hasMicOptIn: false,
    onMicOptIn: vi.fn(),
    onTextChange: vi.fn(),
    onInputModeChange: vi.fn(),
    requestSubmit: vi.fn(),
    ...props,
  };
  return { ...render(<InterviewActiveScreen {...defaultProps} />), props: defaultProps };
}

describe("InterviewActiveScreen", () => {
  beforeEach(() => {
    voiceRecorderMock.mockReset();
    micDialogMock.mockReset();
    answerDraftReadMock.mockReset().mockReturnValue(null);
    answerDraftWriteMock.mockReset();
    answerDraftClearMock.mockReset();
    answerDraftClearStaleMock.mockReset();
    voiceRecorderMock.mockReturnValue(makeVoiceRecorderState());
    micDialogMock.mockReturnValue(makeMicDialogState());
  });

  it("restores draft from localStorage on mount", () => {
    const onTextChange = vi.fn();
    answerDraftReadMock.mockReturnValue({
      turnId: "turn-1",
      questionText: "Question",
      draftText: "Saved draft text",
      source: "text",
      updatedAt: Date.now(),
    });

    renderScreen({ onTextChange });

    expect(answerDraftClearStaleMock).toHaveBeenCalledWith("AP-TEST", "turn-1");
    expect(onTextChange).toHaveBeenCalledWith("Saved draft text");
  });

  it("restores voice_review draft and switches input mode", () => {
    const onTextChange = vi.fn();
    const onInputModeChange = vi.fn();
    answerDraftReadMock.mockReturnValue({
      turnId: "turn-1",
      questionText: "Question",
      draftText: "Transcribed voice",
      source: "voice_review",
      updatedAt: Date.now(),
    });

    renderScreen({ onTextChange, onInputModeChange });

    expect(onTextChange).toHaveBeenCalledWith("Transcribed voice");
    expect(onInputModeChange).toHaveBeenCalledWith("voice");
  });

  it("persists draft to localStorage when textAnswer is non-empty", () => {
    renderScreen({ textAnswer: "My answer" });

    expect(answerDraftWriteMock).toHaveBeenCalledWith(
      "AP-TEST",
      expect.objectContaining({
        turnId: "turn-1",
        draftText: "My answer",
        source: "text",
      }),
    );
  });

  it("clears draft when textAnswer becomes empty", () => {
    renderScreen({ textAnswer: "" });

    expect(answerDraftClearMock).toHaveBeenCalledWith("AP-TEST", "turn-1");
  });

  it("shows timeout dialog when answerSecondsLeft transitions from >0 to 0 on criterion question", () => {
    const { rerender, props } = renderScreen({ answerSecondsLeft: 1, isTimerExpired: false });

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    rerender(
      <InterviewActiveScreen
        {...props}
        answerSecondsLeft={0}
        isTimerExpired={true}
      />,
    );

    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("calls completeVoiceRecording when timer expires during recording", () => {
    const completeVoiceRecording = vi.fn();
    voiceRecorderMock.mockReturnValue(makeVoiceRecorderState({
      voiceRecorderState: "recording",
      isRecordingActive: true,
      completeVoiceRecording,
    }));

    renderScreen({
      inputMode: "voice",
      isTimerExpired: true,
      answerSecondsLeft: 0,
    });

    expect(completeVoiceRecording).toHaveBeenCalledTimes(1);
  });

  it("auto-reviews voice when timer expires with audio_ready", async () => {
    const reviewVoiceRecording = vi.fn(async () => "transcribed text");
    voiceRecorderMock.mockReturnValue(makeVoiceRecorderState({
      voiceRecorderState: "audio_ready",
      voiceBlob: new Blob(["audio"], { type: "audio/webm" }),
      reviewVoiceRecording,
    }));

    const onTextChange = vi.fn();
    renderScreen({
      inputMode: "voice",
      isTimerExpired: true,
      answerSecondsLeft: 0,
      onTextChange,
    });

    await waitFor(() => {
      expect(reviewVoiceRecording).toHaveBeenCalledWith("AP-TEST");
    });
  });

  it("uses the primary voice action to complete and review an active recording", async () => {
    const reviewVoiceRecording = vi.fn(async () => "transcribed text");
    const onTextChange = vi.fn();
    voiceRecorderMock.mockReturnValue(makeVoiceRecorderState({
      voiceRecorderState: "recording",
      isRecordingActive: true,
      reviewVoiceRecording,
    }));

    renderScreen({
      inputMode: "voice",
      onTextChange,
    });

    fireEvent.click(screen.getByRole("button", { name: "Complete and review" }));

    await waitFor(() => {
      expect(reviewVoiceRecording).toHaveBeenCalledWith("AP-TEST");
      expect(onTextChange).toHaveBeenCalledWith("transcribed text");
    });
  });

  it("does NOT show timeout dialog for readiness questions", () => {
    renderScreen({
      currentQuestion: makeQuestion({ kind: "readiness" }),
      answerSecondsLeft: 0,
      isTimerExpired: true,
    });

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("submits answer via timeout dialog submit button", () => {
    const requestSubmit = vi.fn();
    const { rerender, props } = renderScreen({
      answerSecondsLeft: 1,
      isTimerExpired: false,
      textAnswer: "Final answer",
      requestSubmit,
    });

    rerender(
      <InterviewActiveScreen
        {...props}
        answerSecondsLeft={0}
        isTimerExpired={true}
        textAnswer="Final answer"
      />,
    );

    const submitButtons = screen.getAllByRole("button", { name: "Submit answer" });
    fireEvent.click(submitButtons[0]);

    expect(requestSubmit).toHaveBeenCalledWith("Final answer");
  });

  it("renders the answer deadline below the submit button in text mode", () => {
    renderScreen({ textAnswer: "Final answer" });

    const submitButton = screen.getByRole("button", { name: "Submit answer" });
    const timerLabel = screen.getByText("Submit this answer in");

    expect(
      submitButton.compareDocumentPosition(timerLabel) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).not.toBe(0);
  });
});
