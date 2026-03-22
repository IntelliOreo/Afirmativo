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
import type {
  AnswerAsyncAcceptedResponseDto,
  AnswerJobStatusResponseDto,
  InterviewReportDto,
  QuestionDto,
  StartResponseDto,
} from "./dto";
import {
  AnswerAsyncAcceptedResponseSchema,
  AnswerJobStatusResponseSchema,
  InterviewReportSchema,
  parseDto,
  QuestionSchema,
  StartResponseSchema,
} from "./schemas";

export function mapQuestion(raw: QuestionDto): Question {
  const dto = parseDto(QuestionSchema, "QuestionDto", raw);
  return {
    textEs: dto.text_es,
    textEn: dto.text_en,
    area: dto.area,
    kind: dto.kind,
    turnId: toTurnId(dto.turn_id),
    questionNumber: dto.question_number,
    totalQuestions: dto.total_questions,
  };
}

export function mapStartResponse(raw: StartResponseDto): StartInterviewData {
  const dto = parseDto(StartResponseSchema, "StartResponseDto", raw);
  return {
    question: mapQuestion(dto.question),
    timerRemainingS: dto.timer_remaining_s,
    answerSubmitWindowRemainingS: dto.answer_submit_window_remaining_s,
    language: dto.language,
    resuming: dto.resuming,
    error: dto.error,
    code: dto.code,
  };
}

export function mapAnswerJobResponse(raw: AnswerJobStatusResponseDto): AnswerJobStatus {
  const dto = parseDto(AnswerJobStatusResponseSchema, "AnswerJobStatusResponseDto", raw);
  return {
    jobId: toJobId(dto.job_id),
    clientRequestId: toClientRequestId(dto.client_request_id),
    status: dto.status,
    done: dto.done,
    nextQuestion: dto.next_question ? mapQuestion(dto.next_question) : undefined,
    timerRemainingS: dto.timer_remaining_s,
    answerSubmitWindowRemainingS: dto.answer_submit_window_remaining_s,
    errorCode: dto.error_code,
    errorMessage: dto.error_message,
    error: dto.error,
    code: dto.code,
  };
}

export function mapAsyncAcceptedResponse(raw: AnswerAsyncAcceptedResponseDto): AsyncAnswerAccepted {
  const dto = parseDto(AnswerAsyncAcceptedResponseSchema, "AnswerAsyncAcceptedResponseDto", raw);
  return {
    jobId: toJobId(dto.job_id),
    clientRequestId: toClientRequestId(dto.client_request_id),
    status: dto.status,
    error: dto.error,
    code: dto.code,
  };
}

export function mapReport(raw: InterviewReportDto): InterviewReport {
  const dto = parseDto(InterviewReportSchema, "InterviewReportDto", raw);
  return {
    sessionCode: toSessionCode(dto.session_code),
    status: dto.status,
    contentEn: dto.content_en,
    contentEs: dto.content_es,
    areasOfClarity: dto.areas_of_clarity ?? [],
    areasOfClarityEs: dto.areas_of_clarity_es ?? [],
    areasToDevelopFurther: dto.areas_to_develop_further ?? [],
    areasToDevelopFurtherEs: dto.areas_to_develop_further_es ?? [],
    recommendation: dto.recommendation,
    recommendationEs: dto.recommendation_es ?? "",
    questionCount: dto.question_count,
    durationMinutes: dto.duration_minutes,
  };
}
