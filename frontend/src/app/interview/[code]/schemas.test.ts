import { describe, expect, it } from "vitest";
import {
  AnswerAsyncAcceptedResponseSchema,
  AnswerJobStatusResponseSchema,
  ApiValidationError,
  InterviewReportSchema,
  parseDto,
  QuestionSchema,
  StartResponseSchema,
} from "./schemas";

const validQuestion = {
  text_es: "Pregunta",
  text_en: "Question",
  area: "protected_ground",
  kind: "criterion" as const,
  turn_id: "turn-1",
  question_number: 2,
  total_questions: 10,
};

describe("schemas", () => {
  describe("QuestionSchema", () => {
    it("accepts a valid question", () => {
      expect(QuestionSchema.safeParse(validQuestion).success).toBe(true);
    });

    it("rejects missing turn_id", () => {
      const { turn_id: _, ...missing } = validQuestion;
      expect(QuestionSchema.safeParse(missing).success).toBe(false);
    });

    it("rejects invalid kind", () => {
      expect(QuestionSchema.safeParse({ ...validQuestion, kind: "bogus" }).success).toBe(false);
    });

    it("rejects non-numeric question_number", () => {
      expect(QuestionSchema.safeParse({ ...validQuestion, question_number: "two" }).success).toBe(false);
    });
  });

  describe("StartResponseSchema", () => {
    const valid = {
      question: validQuestion,
      timer_remaining_s: 600,
      answer_submit_window_remaining_s: 240,
      language: "es" as const,
    };

    it("accepts a valid start response", () => {
      expect(StartResponseSchema.safeParse(valid).success).toBe(true);
    });

    it("accepts optional fields", () => {
      expect(StartResponseSchema.safeParse({ ...valid, resuming: true, error: "err", code: "C" }).success).toBe(true);
    });

    it("rejects invalid language", () => {
      expect(StartResponseSchema.safeParse({ ...valid, language: "fr" }).success).toBe(false);
    });

    it("rejects missing question", () => {
      const { question: _, ...missing } = valid;
      expect(StartResponseSchema.safeParse(missing).success).toBe(false);
    });
  });

  describe("AnswerAsyncAcceptedResponseSchema", () => {
    const valid = {
      job_id: "job-1",
      client_request_id: "client-1",
      status: "queued" as const,
    };

    it("accepts a valid accepted response", () => {
      expect(AnswerAsyncAcceptedResponseSchema.safeParse(valid).success).toBe(true);
    });

    it("rejects invalid status", () => {
      expect(AnswerAsyncAcceptedResponseSchema.safeParse({ ...valid, status: "pending" }).success).toBe(false);
    });
  });

  describe("AnswerJobStatusResponseSchema", () => {
    const valid = {
      job_id: "job-1",
      client_request_id: "client-1",
      status: "succeeded" as const,
      done: false,
      timer_remaining_s: 540,
      answer_submit_window_remaining_s: 180,
    };

    it("accepts a valid job status response", () => {
      expect(AnswerJobStatusResponseSchema.safeParse(valid).success).toBe(true);
    });

    it("accepts optional next_question", () => {
      expect(AnswerJobStatusResponseSchema.safeParse({ ...valid, next_question: validQuestion }).success).toBe(true);
    });

    it("accepts null next_question", () => {
      expect(AnswerJobStatusResponseSchema.safeParse({ ...valid, next_question: null }).success).toBe(true);
    });

    it("rejects missing done field", () => {
      const { done: _, ...missing } = valid;
      expect(AnswerJobStatusResponseSchema.safeParse(missing).success).toBe(false);
    });
  });

  describe("InterviewReportSchema", () => {
    const valid = {
      session_code: "AP-123",
      status: "ready",
      content_en: "English",
      content_es: "Spanish",
      recommendation: "Practice",
      question_count: 12,
      duration_minutes: 25,
    };

    it("accepts a valid report", () => {
      expect(InterviewReportSchema.safeParse(valid).success).toBe(true);
    });

    it("accepts optional array fields", () => {
      expect(InterviewReportSchema.safeParse({
        ...valid,
        areas_of_clarity: ["a"],
        areas_to_develop_further_es: ["b"],
      }).success).toBe(true);
    });

    it("rejects non-numeric question_count", () => {
      expect(InterviewReportSchema.safeParse({ ...valid, question_count: "twelve" }).success).toBe(false);
    });
  });

  describe("parseDto", () => {
    it("returns parsed data on success", () => {
      const result = parseDto(QuestionSchema, "QuestionDto", validQuestion);
      expect(result.turn_id).toBe("turn-1");
    });

    it("throws ApiValidationError on failure", () => {
      expect(() => parseDto(QuestionSchema, "QuestionDto", { text_es: 123 }))
        .toThrow(ApiValidationError);
    });

    it("includes schema name in error message", () => {
      try {
        parseDto(QuestionSchema, "QuestionDto", {});
      } catch (err) {
        expect(err).toBeInstanceOf(ApiValidationError);
        expect((err as ApiValidationError).message).toContain("QuestionDto");
        expect((err as ApiValidationError).schemaName).toBe("QuestionDto");
        expect((err as ApiValidationError).issues.length).toBeGreaterThan(0);
      }
    });
  });
});
