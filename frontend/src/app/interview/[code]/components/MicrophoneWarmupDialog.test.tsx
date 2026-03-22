import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { MicrophoneWarmupDialog } from "./MicrophoneWarmupDialog";

function makeProps(overrides: Record<string, unknown> = {}) {
  return {
    lang: "en" as const,
    mode: "initial_setup" as const,
    uiState: "idle" as const,
    onEnableMicrophone: vi.fn(async () => {}),
    onDismiss: vi.fn(),
    ...overrides,
  };
}

describe("MicrophoneWarmupDialog", () => {
  it("renders english initial-setup copy", () => {
    render(<MicrophoneWarmupDialog {...makeProps()} />);

    expect(screen.getByText("Prepare the microphone now")).toBeInTheDocument();
    expect(
      screen.getByText("If you plan to answer by voice, enable the microphone now to avoid delay when you record."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Enable microphone" })).toBeInTheDocument();
  });

  it("renders spanish reconnect copy and loading visuals", () => {
    render(<MicrophoneWarmupDialog {...makeProps({
      lang: "es",
      mode: "reconnect",
      uiState: "recovering",
    })}
    />);

    expect(screen.getByText("Volviendo a preparar el micrófono")).toBeInTheDocument();
    expect(screen.getByText("Reconectando el micrófono.")).toBeInTheDocument();
    expect(screen.getByRole("progressbar")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cerrar" })).toBeDisabled();
  });

  it("shows success handoff copy when the mic is ready", () => {
    render(<MicrophoneWarmupDialog {...makeProps({
      uiState: "ready_handoff",
    })}
    />);

    expect(screen.getByText("Your microphone is ready")).toBeInTheDocument();
    expect(screen.getByText("Microphone connected successfully.")).toBeInTheDocument();
    expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuetext", "ready");
  });
});
