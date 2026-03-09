import { describe, expect, it } from "vitest";
import {
  mapAnswerJobResponse,
  mapAsyncAcceptedResponse,
  mapReport,
  mapStartResponse,
} from "./mappers";
import { getQuestionText } from "./models";

describe("interview mappers", () => {
  it("maps report DTO fields to camelCase domain fields", () => {
    const report = mapReport({
      session_code: "AP-123",
      status: "ready",
      content_en: "English content",
      content_es: "Contenido",
      recommendation: "Practice timelines",
      question_count: 12,
      duration_minutes: 25,
    });

    expect(report.sessionCode).toBe("AP-123");
    expect(report.contentEn).toBe("English content");
    expect(report.contentEs).toBe("Contenido");
    expect(report.areasOfClarity).toEqual([]);
    expect(report.areasToDevelopFurther).toEqual([]);
    expect(report.questionCount).toBe(12);
    expect(report.durationMinutes).toBe(25);
  });

  it("maps start and answer job DTOs into branded domain objects", () => {
    const start = mapStartResponse({
      question: {
        text_es: "Pregunta",
        text_en: "Question",
        area: "protected_ground",
        kind: "criterion",
        turn_id: "turn-1",
        question_number: 2,
        total_questions: 10,
      },
      timer_remaining_s: 600,
      answer_submit_window_remaining_s: 240,
      language: "es",
    });
    const accepted = mapAsyncAcceptedResponse({
      job_id: "job-1",
      client_request_id: "client-1",
      status: "queued",
    });
    const status = mapAnswerJobResponse({
      job_id: "job-1",
      client_request_id: "client-1",
      status: "succeeded",
      done: false,
      next_question: {
        text_es: "Siguiente",
        text_en: "Next",
        area: "timeline",
        kind: "criterion",
        turn_id: "turn-2",
        question_number: 3,
        total_questions: 10,
      },
      timer_remaining_s: 540,
      answer_submit_window_remaining_s: 180,
    });

    expect(start.question.turnId).toBe("turn-1");
    expect(getQuestionText(start.question, "es")).toBe("Pregunta");
    expect(accepted.jobId).toBe("job-1");
    expect(accepted.clientRequestId).toBe("client-1");
    expect(status.nextQuestion?.turnId).toBe("turn-2");
    expect(status.timerRemainingS).toBe(540);
    expect(status.answerSubmitWindowRemainingS).toBe(180);
  });
});
