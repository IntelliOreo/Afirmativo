import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { toTurnId } from "../models";
import { InterviewProgressHeader } from "./InterviewProgressHeader";

describe("InterviewProgressHeader", () => {
  it("uses wrapup tone over warning and hides the question counter", () => {
    const { container } = render(
      <InterviewProgressHeader
        lang="en"
        isBlinkingTimer
        isWrapup
        isWarning
        timerLabel="04:59"
        question={{
          textEs: "Pregunta",
          textEn: "Question",
          area: "protected_ground",
          kind: "criterion",
          turnId: toTurnId("turn-1"),
          questionNumber: 3,
          totalQuestions: 25,
        }}
        progressPct={40}
      />,
    );

    const outer = container.firstElementChild;
    const banner = outer?.firstElementChild;

    expect(outer).toHaveClass("animate-pulse");
    expect(banner).toHaveClass("bg-error");
    expect(screen.getByText("Maximum interview time remaining: 04:59")).toBeInTheDocument();
    expect(screen.queryByText("Question 3 / 25")).not.toBeInTheDocument();
  });

  it("uses warning tone when wrapup is false", () => {
    const { container } = render(
      <InterviewProgressHeader
        lang="es"
        isBlinkingTimer={false}
        isWrapup={false}
        isWarning
        timerLabel="10:00"
        question={null}
        progressPct={10}
      />,
    );

    const banner = container.firstElementChild?.firstElementChild;
    expect(banner).toHaveClass("bg-accent-warm");
    expect(screen.getByText("Tiempo máximo restante: 10:00")).toBeInTheDocument();
    expect(screen.queryByText(/Pregunta/)).not.toBeInTheDocument();
  });

  it("uses the default tone when not warning or wrapup", () => {
    const { container } = render(
      <InterviewProgressHeader
        lang="en"
        isBlinkingTimer={false}
        isWrapup={false}
        isWarning={false}
        timerLabel="20:00"
        question={null}
        progressPct={0}
      />,
    );

    const banner = container.firstElementChild?.firstElementChild;
    expect(banner).toHaveClass("bg-primary-dark");
  });
});
