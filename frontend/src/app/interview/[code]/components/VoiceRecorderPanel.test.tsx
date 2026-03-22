import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { VoiceRecorderPanel } from "./VoiceRecorderPanel";

function makeProps(overrides: Record<string, unknown> = {}) {
  return {
    lang: "en" as const,
    hasMicOptIn: false,
    micWarmState: "cold" as const,
    onPrepareMicrophone: vi.fn(async () => {}),
    answerTimerLabel: "02:00",
    answerTimerTone: "normal" as const,
    answerTimerMessage: "Use this time to review and submit your final answer.",
    voiceTimerLabel: "00:00",
    canReplayRecording: false,
    isVoicePreviewPlaying: false,
    onToggleVoicePreviewPlayback: vi.fn(async () => {}),
    voiceIsRecordingActive: false,
    voiceProgressPct: 0,
    voiceWarningRemaining: null,
    voiceReviewWarning: "",
    voiceError: "",
    voiceInfo: "",
    voiceRecorderState: "idle" as const,
    voiceIsRecordingPaused: false,
    voiceBlob: null,
    canDiscardRecording: false,
    onDiscardVoiceRecording: vi.fn(),
    canToggleRecording: true,
    onStartVoiceRecording: vi.fn(async () => {}),
    centerControlLabel: "Record",
    canReviewTranscript: false,
    onReviewVoiceAnswer: vi.fn(async () => {}),
    canSubmitAnswer: false,
    transcriptText: "",
    onTranscriptChange: vi.fn(),
    onSubmitAnswer: vi.fn(),
    ...overrides,
  };
}

describe("VoiceRecorderPanel", () => {
  it("shows a microphone enable CTA when voice mode is open without mic opt-in", () => {
    const onPrepareMicrophone = vi.fn(async () => {});

    render(<VoiceRecorderPanel {...makeProps({ onPrepareMicrophone })} />);

    expect(screen.getByText("Enable microphone")).toBeInTheDocument();
    expect(
      screen.getByText("If you plan to answer by voice, enable the microphone now to avoid delay when you record."),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Enable microphone" }));

    expect(onPrepareMicrophone).toHaveBeenCalledTimes(1);
  });

  it("renders the answer deadline below the primary voice action", () => {
    render(<VoiceRecorderPanel {...makeProps()} />);

    const primaryButton = screen.getByRole("button", { name: "Review transcript" });
    const timerLabel = screen.getByText("Submit this answer in");

    expect(
      primaryButton.compareDocumentPosition(timerLabel) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).not.toBe(0);
  });

  it("moves replay into the bottom control row and removes the timer-adjacent replay button", () => {
    const onToggleVoicePreviewPlayback = vi.fn(async () => {});

    render(<VoiceRecorderPanel
      {...makeProps({
        voiceRecorderState: "audio_ready",
        voiceBlob: new Blob(["audio"]),
        canReplayRecording: true,
        onToggleVoicePreviewPlayback,
      })}
    />);

    expect(screen.getByText("Replay")).toBeInTheDocument();
    expect(screen.getByText("00:00").parentElement?.querySelector("button")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Play recorded audio" }));

    expect(onToggleVoicePreviewPlayback).toHaveBeenCalledTimes(1);
  });

  it("shows complete and review as the primary action while recording", () => {
    render(<VoiceRecorderPanel
      {...makeProps({
        voiceRecorderState: "recording",
        voiceIsRecordingActive: true,
        canReviewTranscript: true,
      })}
    />);

    expect(screen.getByRole("button", { name: "Complete and review" })).toBeInTheDocument();
  });
});
