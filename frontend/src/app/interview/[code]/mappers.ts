import type {
  AnswerAsyncAcceptedResponseDto,
  AnswerJobStatusResponseDto,
  InterviewReportDto,
  QuestionDto,
  StartResponseDto,
} from "./dto";
import type {
  AnswerJobStatus,
  AsyncAnswerAccepted,
  InterviewReport,
  Question,
  StartInterviewData,
} from "./models";
import {
  toClientRequestId,
  toJobId,
  toSessionCode,
  toTurnId,
} from "./models";

export function mapQuestion(raw: QuestionDto): Question {
  return {
    textEs: raw.text_es,
    textEn: raw.text_en,
    area: raw.area,
    kind: raw.kind,
    turnId: toTurnId(raw.turn_id),
    questionNumber: raw.question_number,
    totalQuestions: raw.total_questions,
  };
}

export function mapStartResponse(raw: StartResponseDto): StartInterviewData {
  return {
    question: mapQuestion(raw.question),
    timerRemainingS: raw.timer_remaining_s,
    language: raw.language,
    error: raw.error,
    code: raw.code,
  };
}

export function mapAnswerJobResponse(raw: AnswerJobStatusResponseDto): AnswerJobStatus {
  return {
    jobId: toJobId(raw.job_id),
    clientRequestId: toClientRequestId(raw.client_request_id),
    status: raw.status,
    done: raw.done,
    nextQuestion: raw.next_question ? mapQuestion(raw.next_question) : undefined,
    timerRemainingS: raw.timer_remaining_s,
    errorCode: raw.error_code,
    errorMessage: raw.error_message,
    error: raw.error,
    code: raw.code,
  };
}

export function mapAsyncAcceptedResponse(raw: AnswerAsyncAcceptedResponseDto): AsyncAnswerAccepted {
  return {
    jobId: toJobId(raw.job_id),
    clientRequestId: toClientRequestId(raw.client_request_id),
    status: raw.status,
    error: raw.error,
    code: raw.code,
  };
}

export function mapReport(raw: InterviewReportDto): InterviewReport {
  return {
    sessionCode: toSessionCode(raw.session_code),
    status: raw.status,
    contentEn: raw.content_en,
    contentEs: raw.content_es,
    areasOfClarity: raw.areas_of_clarity ?? [],
    areasToDevelopFurther: raw.areas_to_develop_further ?? [],
    recommendation: raw.recommendation,
    questionCount: raw.question_count,
    durationMinutes: raw.duration_minutes,
  };
}
