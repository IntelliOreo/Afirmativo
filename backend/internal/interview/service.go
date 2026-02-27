// Service layer for interview operations.
// StartInterview: sets session active, returns first question.
// SubmitAnswer: text answer → AI eval → next question.
// SubmitAudioAnswer: audio → Whisper → Translate → AI eval → next question + transcript.
// EndInterview: force-end on timer expiry, triggers report generation.
package interview
