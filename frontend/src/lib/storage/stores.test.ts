import { describe, expect, it } from "vitest";
import {
  readInterviewLang,
  readUiLang,
  writeInterviewLang,
  writeUiLang,
} from "./languageStore";
import { read as readPendingAnswer, write as writePendingAnswer } from "./pendingAnswerStore";
import { readAndConsumePin, writePin } from "./sessionPinStore";
import {
  toClientRequestId,
  toJobId,
  toTurnId,
} from "@/app/interview/[code]/models";

describe("languageStore", () => {
  it("round-trips ui and interview-scoped languages", () => {
    writeUiLang("en");
    writeInterviewLang("AP-123", "es");

    expect(readUiLang()).toBe("en");
    expect(readInterviewLang("AP-123")).toBe("es");
  });
});

describe("sessionPinStore", () => {
  it("reads and consumes stored pins", () => {
    writePin("AP-123", " 1234 ");

    expect(readAndConsumePin("AP-123")).toBe("1234");
    expect(readAndConsumePin("AP-123")).toBeNull();
  });
});

describe("pendingAnswerStore", () => {
  it("round-trips valid pending jobs", () => {
    writePendingAnswer("AP-123", {
      clientRequestId: toClientRequestId("client-1"),
      turnId: toTurnId("turn-1"),
      answerText: "Answer",
      questionText: "Question",
      jobId: toJobId("job-1"),
      createdAt: 123,
    });

    expect(readPendingAnswer("AP-123")).toEqual({
      clientRequestId: "client-1",
      turnId: "turn-1",
      answerText: "Answer",
      questionText: "Question",
      jobId: "job-1",
      createdAt: 123,
    });
  });

  it("clears malformed persisted jobs", () => {
    window.localStorage.setItem("interview_pending_answer_job_AP-123", "{not-json");

    expect(readPendingAnswer("AP-123")).toBeNull();
    expect(window.localStorage.getItem("interview_pending_answer_job_AP-123")).toBeNull();
  });
});
