import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import InterviewPage from "./page";

const useParamsMock = vi.fn(() => ({ code: "AP-123" }));
const useSearchParamsMock = vi.fn(() => ({ get: () => "en" }));
const loadReportMock = vi.fn(async () => {});
const resumeReportMock = vi.fn(async () => {});
const printReportMock = vi.fn();
const machineMock = vi.fn();
const voiceRecorderMock = vi.fn();
const questionCardRenderMock = vi.fn();
const voiceAnswerSectionRenderMock = vi.fn();

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
    resumeReport: resumeReportMock,
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

vi.mock("./components/InterviewQuestionCard", async () => {
  const React = await import("react");

  return {
    InterviewQuestionCard: React.memo(({ questionText }: { questionText: string }) => {
      questionCardRenderMock(questionText);
      return <div>{questionText}</div>;
    }),
  };
});

vi.mock("./components/VoiceAnswerSection", async () => {
  const React = await import("react");

  return {
    VoiceAnswerSection: React.memo(({ voiceDurationSeconds }: { voiceDurationSeconds: number }) => {
      voiceAnswerSectionRenderMock(voiceDurationSeconds);
      return <div>Voice section {voiceDurationSeconds}</div>;
    }),
  };
});

function makeVoiceRecorderState(overrides: Record<string, unknown> = {}) {
  return {
    voiceRecorderState: "idle",
    micWarmState: "cold",
    voiceDurationSeconds: 0,
    voiceWarningSeconds: null,
    voiceBlob: null,
    voicePreviewUrl: null,
    isVoicePreviewPlaying: false,
    voiceError: "",
    voiceInfo: "",
    isRecordingActive: false,
    isRecordingPaused: false,
    prepareMicrophone: vi.fn(async () => false),
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
    retryPendingRecovery: vi.fn(),
    canRetryPendingRecovery: false,
  };
}

describe("InterviewPage", () => {
  beforeEach(() => {
    loadReportMock.mockReset();
    resumeReportMock.mockReset();
    printReportMock.mockReset();
    machineMock.mockReset();
    voiceRecorderMock.mockReset();
    questionCardRenderMock.mockReset();
    voiceAnswerSectionRenderMock.mockReset();
    machineMock.mockReturnValue(makeMachineState({ phase: "guard" }));
    voiceRecorderMock.mockReturnValue(makeVoiceRecorderState());
    vi.spyOn(window, "confirm").mockReturnValue(true);
  });

  afterEach(() => {
    vi.useRealTimers();
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

  it("keeps the question shell stable when only voice duration changes", async () => {
    let voiceDurationSeconds = 0;
    machineMock.mockReturnValue(
      makeMachineState({
        phase: "active",
        question: activeQuestion,
        secondsLeft: 600,
        answerSecondsLeft: 240,
        textAnswer: "",
        inputMode: "voice",
      }),
    );
    voiceRecorderMock.mockImplementation(() => makeVoiceRecorderState({
      voiceRecorderState: "recording",
      isRecordingActive: true,
      voiceDurationSeconds,
    }));

    const { rerender } = render(<InterviewPage />);

    await waitFor(() => {
      expect(screen.getByText("Active question")).toBeInTheDocument();
      expect(screen.getByText("Voice section 0")).toBeInTheDocument();
    });

    voiceDurationSeconds = 1;
    rerender(<InterviewPage />);

    await waitFor(() => {
      expect(screen.getByText("Voice section 1")).toBeInTheDocument();
    });

    expect(questionCardRenderMock).toHaveBeenCalledTimes(1);
    expect(voiceAnswerSectionRenderMock).toHaveBeenCalledTimes(2);
  });

  it("shows the readiness microphone prompt and enables session-level keep-warm after opt-in", async () => {
    vi.useFakeTimers();
    const prepareMicrophoneMock = vi.fn(async () => true);
    machineMock.mockReturnValue(
      makeMachineState({
        phase: "active",
        question: {
          ...activeQuestion,
          kind: "readiness",
          turnId: "turn-ready",
        },
        secondsLeft: 600,
        answerSecondsLeft: 240,
        textAnswer: "",
        inputMode: "text",
      }),
    );
    voiceRecorderMock.mockReturnValue(makeVoiceRecorderState({
      prepareMicrophone: prepareMicrophoneMock,
      micWarmState: "cold",
    }));

    render(<InterviewPage />);

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Prepare the microphone now")).toBeInTheDocument();
    expect(voiceRecorderMock.mock.calls.at(-1)?.[0]).toMatchObject({
      shouldKeepMicWarm: false,
    });

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Enable microphone" }));
      await Promise.resolve();
    });

    expect(prepareMicrophoneMock).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Your microphone is ready")).toBeInTheDocument();
    expect(voiceRecorderMock.mock.calls.at(-1)?.[0]).toMatchObject({
      shouldKeepMicWarm: false,
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(voiceRecorderMock.mock.calls.at(-1)?.[0]).toMatchObject({
      shouldKeepMicWarm: true,
    });
  });

  it("keeps voice mode rendering separate from session-level mic warmup", async () => {
    machineMock.mockReturnValue(
      makeMachineState({
        phase: "active",
        question: activeQuestion,
        secondsLeft: 600,
        answerSecondsLeft: 240,
        textAnswer: "",
        inputMode: "voice",
      }),
    );

    render(<InterviewPage />);

    await waitFor(() => {
      expect(screen.getByText("Voice section 0")).toBeInTheDocument();
    });

    expect(voiceRecorderMock.mock.calls.at(-1)?.[0]).toMatchObject({
      shouldKeepMicWarm: false,
    });
  });

  it("allows the readiness microphone prompt to be dismissed without warming the mic", async () => {
    machineMock.mockReturnValue(
      makeMachineState({
        phase: "active",
        question: {
          ...activeQuestion,
          kind: "readiness",
          turnId: "turn-ready-dismiss",
        },
        secondsLeft: 600,
        answerSecondsLeft: 240,
        textAnswer: "",
        inputMode: "text",
      }),
    );

    render(<InterviewPage />);

    await waitFor(() => {
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Not now" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(voiceRecorderMock.mock.calls.at(-1)?.[0]).toMatchObject({
      shouldKeepMicWarm: false,
    });
  });

  it("shows the reconnect dialog only after the reveal delay when the warm stream goes stale", async () => {
    vi.useFakeTimers();
    let micWarmState = "cold";
    const prepareMicrophoneMock = vi.fn(async () => true);
    machineMock.mockReturnValue(
      makeMachineState({
        phase: "active",
        question: {
          ...activeQuestion,
          kind: "readiness",
          turnId: "turn-ready-reconnect",
        },
        secondsLeft: 600,
        answerSecondsLeft: 240,
        textAnswer: "",
        inputMode: "text",
      }),
    );
    voiceRecorderMock.mockImplementation(() => makeVoiceRecorderState({
      prepareMicrophone: prepareMicrophoneMock,
      micWarmState,
    }));

    const { rerender } = render(<InterviewPage />);

    expect(screen.getByRole("dialog")).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Enable microphone" }));
      await Promise.resolve();
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    micWarmState = "recovering";
    rerender(<InterviewPage />);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(299);
    });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Preparing the microphone again")).toBeInTheDocument();
  });
});
