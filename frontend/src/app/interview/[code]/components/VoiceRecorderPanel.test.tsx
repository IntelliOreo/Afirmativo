import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { VoiceRecorderPanel } from "./VoiceRecorderPanel";

function renderPanel(
  props: Partial<React.ComponentProps<typeof VoiceRecorderPanel>> = {},
) {
  return render(
    <VoiceRecorderPanel
      lang="en"
      answerTimerLabel="04:00"
      answerTimerTone="normal"
      answerTimerMessage="Use this time to review and submit your final answer."
      voiceTimerLabel="00:12"
      canPreviewRecording={false}
      isVoicePreviewPlaying={false}
      onToggleVoicePreviewPlayback={vi.fn(async () => {})}
      voiceIsRecordingActive={false}
      voiceProgressPct={10}
      voiceWarningRemaining={null}
      voiceReviewWarning=""
      voiceError=""
      voiceInfo=""
      voiceRecorderState="idle"
      voiceIsRecordingPaused={false}
      voiceBlob={null}
      canDiscardRecording={false}
      onDiscardVoiceRecording={vi.fn()}
      canToggleRecording
      onStartVoiceRecording={vi.fn(async () => {})}
      centerControlLabel="Record"
      canCompleteRecording={false}
      onCompleteVoiceRecording={vi.fn()}
      canReviewTranscript={false}
      onReviewVoiceAnswer={vi.fn(async () => {})}
      canSubmitAnswer={false}
      transcriptText=""
      onTranscriptChange={vi.fn()}
      onSubmitAnswer={vi.fn(async () => {})}
      {...props}
    />,
  );
}

describe("VoiceRecorderPanel", () => {
  it("shows the idle message and disabled non-record actions by default", () => {
    renderPanel();

    expect(screen.getByText("Press Record to begin.")).toBeInTheDocument();
    const buttons = screen.getAllByRole("button");
    expect(buttons[0]).toBeDisabled();
    expect(buttons[1]).toBeDisabled();
    expect(buttons[2]).toBeEnabled();
    expect(buttons[3]).toBeDisabled();
    expect(buttons[4]).toBeDisabled();
  });

  it("shows paused messaging and exposes preview, discard, complete, and send when provided", () => {
    renderPanel({
      voiceRecorderState: "review_ready",
      voiceIsRecordingPaused: true,
      voiceBlob: new Blob(["audio"]),
      canPreviewRecording: true,
      canDiscardRecording: true,
      canCompleteRecording: false,
      canSubmitAnswer: true,
      centerControlLabel: "Resume",
      transcriptText: "Transcript ready",
    });

    expect(screen.getByText("Recording paused.")).toBeInTheDocument();
    expect(screen.getByText(/Audio ready:/)).toBeInTheDocument();
    const buttons = screen.getAllByRole("button");
    expect(buttons[0]).toBeEnabled();
    expect(buttons[1]).toBeEnabled();
    expect(buttons[3]).toBeDisabled();
    expect(buttons[4]).toBeEnabled();
  });

  it("shows the active recording message and warning banner", () => {
    renderPanel({
      voiceRecorderState: "recording",
      voiceIsRecordingActive: true,
      voiceWarningRemaining: 12,
      canDiscardRecording: true,
      canCompleteRecording: true,
      centerControlLabel: "Pause",
    });

    expect(screen.getByText("Recording...")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("12s remain before the limit of 3 minutes.");
  });

  it("shows the completed message in spanish when recording is ready to send", () => {
    render(
      <VoiceRecorderPanel
        lang="es"
        answerTimerLabel="04:00"
        answerTimerTone="normal"
        answerTimerMessage="Use este tiempo para revisar y enviar su respuesta final."
        voiceTimerLabel="00:12"
        canPreviewRecording
        isVoicePreviewPlaying={false}
        onToggleVoicePreviewPlayback={vi.fn(async () => {})}
        voiceIsRecordingActive={false}
        voiceProgressPct={10}
        voiceWarningRemaining={null}
        voiceReviewWarning=""
        voiceError=""
        voiceInfo=""
        voiceRecorderState="audio_ready"
        voiceIsRecordingPaused={false}
        voiceBlob={new Blob(["audio"])}
        canDiscardRecording
        onDiscardVoiceRecording={vi.fn()}
        canToggleRecording={false}
        onStartVoiceRecording={vi.fn(async () => {})}
        centerControlLabel="Resume"
        canCompleteRecording={false}
        onCompleteVoiceRecording={vi.fn()}
        canReviewTranscript
        onReviewVoiceAnswer={vi.fn(async () => {})}
        canSubmitAnswer={false}
        transcriptText=""
        onTranscriptChange={vi.fn()}
        onSubmitAnswer={vi.fn(async () => {})}
      />,
    );

    expect(screen.getByText("Audio listo. Revise la transcripción antes de enviar.")).toBeInTheDocument();
  });
});
