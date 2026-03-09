import type { Lang } from "@/lib/language";

export interface QuestionDto {
  text_es: string;
  text_en: string;
  area: string;
  kind: "disclaimer" | "readiness" | "criterion";
  turn_id: string;
  question_number: number;
  total_questions: number;
}

export interface AnswerResponseDto {
  done: boolean;
  next_question?: QuestionDto;
  timer_remaining_s: number;
  answer_submit_window_remaining_s: number;
  error?: string;
  code?: string;
}

export interface StartResponseDto {
  question: QuestionDto;
  timer_remaining_s: number;
  answer_submit_window_remaining_s: number;
  language: Lang;
  resuming?: boolean;
  error?: string;
  code?: string;
}

export type AsyncAnswerJobStatus =
  | "queued"
  | "running"
  | "succeeded"
  | "failed"
  | "conflict"
  | "canceled";

export interface AnswerAsyncAcceptedResponseDto {
  job_id: string;
  client_request_id: string;
  status: AsyncAnswerJobStatus;
  error?: string;
  code?: string;
}

export interface AnswerJobStatusResponseDto {
  job_id: string;
  client_request_id: string;
  status: AsyncAnswerJobStatus;
  done: boolean;
  next_question?: QuestionDto;
  timer_remaining_s: number;
  answer_submit_window_remaining_s: number;
  error_code?: string;
  error_message?: string;
  error?: string;
  code?: string;
}

export interface InterviewReportDto {
  session_code: string;
  status: string;
  content_en: string;
  content_es: string;
  areas_of_clarity?: string[];
  areas_to_develop_further?: string[];
  recommendation: string;
  question_count: number;
  duration_minutes: number;
}
