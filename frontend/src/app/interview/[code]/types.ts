export type Lang = "es" | "en";

export type InterviewStatus = "guard" | "loading" | "active" | "submitting" | "done" | "error";
export type ReportStatus = "idle" | "loading" | "generating" | "ready" | "error";
export type CompletionSource = "finished" | "already_completed";
export type InputMode = "text" | "voice";
export type VoiceRecorderState = "idle" | "recording" | "paused" | "stopped" | "sending";

export interface Question {
  textEs: string;
  textEn: string;
  area: string;
  kind: "disclaimer" | "readiness" | "criterion";
  turnId: string;
  questionNumber: number;
  totalQuestions: number;
}

export interface AnswerResponse {
  done: boolean;
  nextQuestion?: Question;
  timerRemainingS: number;
  error?: string;
  code?: string;
}

export interface StartResponse {
  question: Question;
  timerRemainingS: number;
  language: Lang;
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

export interface AnswerAsyncAcceptedResponse {
  jobId: string;
  clientRequestId: string;
  status: AsyncAnswerJobStatus;
  error?: string;
  code?: string;
}

export interface AnswerJobStatusResponse {
  jobId: string;
  clientRequestId: string;
  status: AsyncAnswerJobStatus;
  done: boolean;
  nextQuestion?: Question;
  timerRemainingS: number;
  errorCode?: string;
  errorMessage?: string;
  error?: string;
  code?: string;
}

export interface PendingAnswerJob {
  clientRequestId: string;
  turnId: string;
  answerText: string;
  questionText: string;
  jobId?: string;
  createdAt: number;
}

export type CodedError = Error & { code?: string };

export interface Report {
  session_code: string;
  status: string;
  content_en: string;
  content_es: string;
  areas_of_clarity: string[];
  areas_to_develop_further: string[];
  recommendation: string;
  question_count: number;
  duration_minutes: number;
}

export type DisclaimerBlock =
  | { type: "paragraph"; text: string }
  | { type: "list"; items: string[] };
