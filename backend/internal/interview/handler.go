// HTTP handlers for interview endpoints:
//   POST /api/interview/start       — HandleStart
//   POST /api/interview/answer      — HandleAnswer (text, JSON, 10KB limit)
//   POST /api/interview/answer-audio — HandleAnswerAudio (multipart, 2MB limit)
//   POST /api/interview/end         — HandleEnd
package interview
