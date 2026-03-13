import { z } from "zod";

export class ApiValidationError extends Error {
  constructor(
    public readonly schemaName: string,
    public readonly issues: z.core.$ZodIssue[],
  ) {
    const summary = issues
      .slice(0, 3)
      .map((i) => `${i.path?.join(".") || "(root)"}: ${i.message}`)
      .join("; ");
    super(`API response validation failed [${schemaName}]: ${summary}`);
    this.name = "ApiValidationError";
  }
}

export function parseDto<T>(schema: z.ZodType<T>, schemaName: string, raw: unknown): T {
  const result = schema.safeParse(raw);
  if (!result.success) {
    throw new ApiValidationError(schemaName, result.error.issues);
  }
  return result.data;
}

export const QuestionSchema = z.object({
  text_es: z.string(),
  text_en: z.string(),
  area: z.string(),
  kind: z.enum(["disclaimer", "readiness", "criterion"]),
  turn_id: z.string(),
  question_number: z.number(),
  total_questions: z.number(),
});

export const StartResponseSchema = z.object({
  question: QuestionSchema,
  timer_remaining_s: z.number(),
  answer_submit_window_remaining_s: z.number(),
  language: z.enum(["es", "en"]),
  resuming: z.boolean().optional(),
  error: z.string().optional(),
  code: z.string().optional(),
});

const AsyncAnswerJobStatusSchema = z.enum([
  "queued",
  "running",
  "succeeded",
  "failed",
  "conflict",
  "canceled",
]);

export const AnswerAsyncAcceptedResponseSchema = z.object({
  job_id: z.string(),
  client_request_id: z.string(),
  status: AsyncAnswerJobStatusSchema,
  error: z.string().optional(),
  code: z.string().optional(),
});

export const AnswerJobStatusResponseSchema = z.object({
  job_id: z.string(),
  client_request_id: z.string(),
  status: AsyncAnswerJobStatusSchema,
  done: z.boolean(),
  next_question: QuestionSchema.optional(),
  timer_remaining_s: z.number(),
  answer_submit_window_remaining_s: z.number(),
  error_code: z.string().optional(),
  error_message: z.string().optional(),
  error: z.string().optional(),
  code: z.string().optional(),
});

export const InterviewReportSchema = z.object({
  session_code: z.string(),
  status: z.string(),
  content_en: z.string(),
  content_es: z.string(),
  areas_of_clarity: z.array(z.string()).optional(),
  areas_of_clarity_es: z.array(z.string()).optional(),
  areas_to_develop_further: z.array(z.string()).optional(),
  areas_to_develop_further_es: z.array(z.string()).optional(),
  recommendation: z.string(),
  recommendation_es: z.string().optional(),
  question_count: z.number(),
  duration_minutes: z.number(),
});
