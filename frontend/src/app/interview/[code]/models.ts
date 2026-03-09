import type { Lang } from "@/lib/language";
import type { AsyncAnswerJobStatus } from "./dto";

type Brand<Value, Name extends string> = Value & { readonly __brand: Name };

export type SessionCode = Brand<string, "SessionCode">;
export type TurnId = Brand<string, "TurnId">;
export type JobId = Brand<string, "JobId">;
export type ClientRequestId = Brand<string, "ClientRequestId">;

export interface Question {
  textEs: string;
  textEn: string;
  area: string;
  kind: "disclaimer" | "readiness" | "criterion";
  turnId: TurnId;
  questionNumber: number;
  totalQuestions: number;
}

export interface AnswerOutcome {
  done: boolean;
  nextQuestion?: Question;
  timerRemainingS: number;
  error?: string;
  code?: string;
}

export interface StartInterviewData {
  question: Question;
  timerRemainingS: number;
  language: Lang;
  error?: string;
  code?: string;
}

export interface AsyncAnswerAccepted {
  jobId: JobId;
  clientRequestId: ClientRequestId;
  status: AsyncAnswerJobStatus;
  error?: string;
  code?: string;
}

export interface AnswerJobStatus {
  jobId: JobId;
  clientRequestId: ClientRequestId;
  status: AsyncAnswerJobStatus;
  done: boolean;
  nextQuestion?: Question;
  timerRemainingS: number;
  errorCode?: string;
  errorMessage?: string;
  error?: string;
  code?: string;
}

export interface PendingAnswerSubmission {
  clientRequestId: ClientRequestId;
  turnId: TurnId;
  answerText: string;
  questionText: string;
  jobId?: JobId;
  createdAt: number;
}

export interface InterviewReport {
  sessionCode: SessionCode;
  status: string;
  contentEn: string;
  contentEs: string;
  areasOfClarity: string[];
  areasToDevelopFurther: string[];
  recommendation: string;
  questionCount: number;
  durationMinutes: number;
}

export function toSessionCode(value: string): SessionCode {
  return value as SessionCode;
}

export function toTurnId(value: string): TurnId {
  return value as TurnId;
}

export function toJobId(value: string): JobId {
  return value as JobId;
}

export function toClientRequestId(value: string): ClientRequestId {
  return value as ClientRequestId;
}

export function getQuestionText(question: Question | null | undefined, lang: Lang): string {
  if (!question) return "";
  return lang === "es"
    ? question.textEs || question.textEn || ""
    : question.textEn || question.textEs || "";
}
