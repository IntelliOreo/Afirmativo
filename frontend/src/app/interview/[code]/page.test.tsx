import { render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import InterviewPage from "./page";

const useParamsMock = vi.fn(() => ({ code: "AP-123" }));
const useSearchParamsMock = vi.fn(() => ({ get: () => "en" }));
const loadReportMock = vi.fn(async () => {});
const printReportMock = vi.fn();
const machineMock = vi.fn();
const voiceRecorderMock = vi.fn();

vi.mock("next/navigation", () => ({
  useParams: () => useParamsMock(),
  useSearchParams: () => useSearchParamsMock(),
}));

vi.mock("next/link", () => ({
  default: ({ href, children }: { href: string; children: React.ReactNode }) => (
    <a href={href}>{children}</a>
  ),
}));

vi.mock("./hooks/useInterviewLanguage", () => ({
  useInterviewLanguage: () => ({
    lang: "en",
    setLang: vi.fn(),
    langInitialized: true,
  }),
}));

vi.mock("./hooks/useInterviewReport", () => ({
  useInterviewReport: () => ({
    reportStatus: "idle",
    report: null,
    reportError: "",
    loadReport: loadReportMock,
    printReport: printReportMock,
  }),
}));

vi.mock("./hooks/useDisclaimerScrollGate", () => ({
  useDisclaimerScrollGate: () => ({
    disclaimerScrollRef: { current: null },
    hasReachedDisclaimerBottom: true,
    updateDisclaimerScrollState: vi.fn(),
  }),
}));

vi.mock("./hooks/useInterviewWaitingStatus", () => ({
  useInterviewWaitingStatus: () => ({
    startupWaitStatus: "",
    questionWaitStatus: "",
    finalSubmitWaitStatus: "",
    reportWaitStatus: "",
  }),
}));

vi.mock("./hooks/useVoiceRecorder", () => ({
  useVoiceRecorder: (...args: unknown[]) => voiceRecorderMock(...args),
}));

vi.mock("./hooks/useInterviewMachine", () => ({
  useInterviewMachine: (...args: unknown[]) => machineMock(...args),
}));

function makeVoiceRecorderState(overrides: Record<string, unknown> = {}) {
  return {
    voiceRecorderState: "idle",
    voiceDurationSeconds: 0,
    voiceWarningSeconds: null,
    voiceBlob: null,
    voicePreviewUrl: null,
    isVoicePreviewPlaying: false,
    voiceError: "",
    voiceInfo: "",
    isRecordingActive: false,
    isRecordingPaused: false,
    startVoiceRecording: vi.fn(async () => {}),
    completeVoiceRecording: vi.fn(),
    discardVoiceRecording: vi.fn(),
    toggleVoicePreviewPlayback: vi.fn(async () => {}),
    reviewVoiceRecording: vi.fn(async () => null),
    finalizeVoiceRecording: vi.fn(async () => null),
    setVoiceErrorMessage: vi.fn(),
    ...overrides,
  };
}

const activeQuestion = {
  textEs: "Pregunta activa",
  textEn: "Active question",
  area: "protected_ground",
  kind: "criterion" as const,
  turnId: "turn-1",
  questionNumber: 2,
  totalQuestions: 25,
};

function makeMachineState(state: Record<string, unknown>) {
  return {
    state,
    dispatch: vi.fn(),
    requestSubmit: vi.fn(),
  };
}

describe("InterviewPage", () => {
  beforeEach(() => {
    loadReportMock.mockReset();
    printReportMock.mockReset();
    machineMock.mockReset();
    voiceRecorderMock.mockReset();
    machineMock.mockReturnValue(makeMachineState({ phase: "guard" }));
    voiceRecorderMock.mockReturnValue(makeVoiceRecorderState());
    vi.spyOn(window, "confirm").mockReturnValue(true);
  });

  it("renders the guard state for unauthorized interview starts", async () => {
    machineMock.mockReturnValue(makeMachineState({ phase: "guard" }));

    render(<InterviewPage />);

    await waitFor(() => expect(screen.getByText("Session not found")).toBeInTheDocument());
    expect(screen.getByRole("link", { name: "Recover session" })).toHaveAttribute("href", "/session/AP-123");
  });

  it("renders the completed flow when the interview is already finished", async () => {
    machineMock.mockReturnValue(
      makeMachineState({ phase: "done", completionSource: "already_completed" }),
    );

    render(<InterviewPage />);

    await waitFor(() => expect(screen.getByText("Interview completed")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Generate report" })).toBeInTheDocument();
  });

  it("renders the error state for generic start failures", async () => {
    machineMock.mockReturnValue(
      makeMachineState({ phase: "error", message: "Backend exploded", code: "" }),
    );

    render(<InterviewPage />);

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("Error: Backend exploded"));
  });

  it("disables input mode switching while the voice recorder is active", async () => {
    machineMock.mockReturnValue(
      makeMachineState({
        phase: "active",
        question: activeQuestion,
        secondsLeft: 600,
        answerSecondsLeft: 240,
        textAnswer: "",
        inputMode: "text",
      }),
    );
    voiceRecorderMock.mockReturnValue(makeVoiceRecorderState({
      voiceRecorderState: "recording",
      isRecordingActive: true,
    }));

    render(<InterviewPage />);

    await waitFor(() => {
      expect(screen.getByText("Active question")).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Text input" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Voice input" })).toBeDisabled();
  });

  it("leaves input mode switching enabled when the recorder is idle", async () => {
    machineMock.mockReturnValue(
      makeMachineState({
        phase: "active",
        question: activeQuestion,
        secondsLeft: 600,
        answerSecondsLeft: 240,
        textAnswer: "",
        inputMode: "text",
      }),
    );
    voiceRecorderMock.mockReturnValue(makeVoiceRecorderState({
      voiceRecorderState: "idle",
    }));

    render(<InterviewPage />);

    await waitFor(() => {
      expect(screen.getByText("Active question")).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Text input" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Voice input" })).toBeEnabled();
  });
});
