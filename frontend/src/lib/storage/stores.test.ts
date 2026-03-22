import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { write as writeAnswerDraft } from "./answerDraftStore";
import { clearAllInterviewStorage } from "./interviewStorage";
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

function quotaExceededError(): DOMException {
  return new DOMException("Quota exceeded", "QuotaExceededError");
}

beforeEach(() => {
  window.localStorage.clear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

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

  it("clears all interview-scoped storage while preserving unrelated keys", () => {
    window.localStorage.setItem("interview_answer_draft_OLD_turn-1", "draft");
    window.localStorage.setItem("interview_pending_answer_job_OLD", "pending");
    window.localStorage.setItem("unrelated_key", "keep");

    clearAllInterviewStorage();

    expect(window.localStorage.getItem("interview_answer_draft_OLD_turn-1")).toBeNull();
    expect(window.localStorage.getItem("interview_pending_answer_job_OLD")).toBeNull();
    expect(window.localStorage.getItem("unrelated_key")).toBe("keep");
  });

  it("clears interview storage and retries once when draft writes hit quota", () => {
    window.localStorage.setItem("interview_answer_draft_OLD_turn-1", JSON.stringify({
      turnId: "turn-1",
      questionText: "Old question",
      draftText: "Old draft",
      source: "text",
      updatedAt: 1,
    }));
    window.localStorage.setItem("interview_pending_answer_job_OLD", JSON.stringify({
      clientRequestId: "client-old",
      turnId: "turn-old",
      answerText: "Old answer",
      questionText: "Old question",
      createdAt: 1,
    }));
    window.localStorage.setItem("unrelated_key", "keep");

    const originalSetItem = window.localStorage.setItem.bind(window.localStorage);
    let shouldThrow = true;
    const setItemSpy = vi.spyOn(window.localStorage, "setItem").mockImplementation(function(
      key: string,
      value: string,
    ) {
      if (shouldThrow && key === "interview_answer_draft_AP-123_turn-2") {
        shouldThrow = false;
        throw quotaExceededError();
      }
      return originalSetItem(key, value);
    });
    const removeItemSpy = vi.spyOn(window.localStorage, "removeItem");

    writeAnswerDraft("AP-123", {
      turnId: "turn-2",
      questionText: "Question",
      draftText: "Draft",
      source: "text",
      updatedAt: 123,
    });

    expect(JSON.parse(window.localStorage.getItem("interview_answer_draft_AP-123_turn-2") ?? "null")).toEqual({
      turnId: "turn-2",
      questionText: "Question",
      draftText: "Draft",
      source: "text",
      updatedAt: 123,
    });
    expect(window.localStorage.getItem("unrelated_key")).toBe("keep");
    expect(removeItemSpy).toHaveBeenCalledWith("interview_answer_draft_OLD_turn-1");
    expect(removeItemSpy).toHaveBeenCalledWith("interview_pending_answer_job_OLD");
    expect(setItemSpy).toHaveBeenCalledTimes(2);
  });

  it("clears interview storage and retries once when pending writes hit quota", () => {
    window.localStorage.setItem("interview_answer_draft_OLD_turn-1", JSON.stringify({
      turnId: "turn-1",
      questionText: "Old question",
      draftText: "Old draft",
      source: "text",
      updatedAt: 1,
    }));
    window.localStorage.setItem("interview_pending_answer_job_OLD", JSON.stringify({
      clientRequestId: "client-old",
      turnId: "turn-old",
      answerText: "Old answer",
      questionText: "Old question",
      createdAt: 1,
    }));
    window.localStorage.setItem("unrelated_key", "keep");

    const originalSetItem = window.localStorage.setItem.bind(window.localStorage);
    let shouldThrow = true;
    const setItemSpy = vi.spyOn(window.localStorage, "setItem").mockImplementation(function(
      key: string,
      value: string,
    ) {
      if (shouldThrow && key === "interview_pending_answer_job_AP-123") {
        shouldThrow = false;
        throw quotaExceededError();
      }
      return originalSetItem(key, value);
    });
    const removeItemSpy = vi.spyOn(window.localStorage, "removeItem");

    writePendingAnswer("AP-123", {
      clientRequestId: toClientRequestId("client-1"),
      turnId: toTurnId("turn-1"),
      answerText: "Answer",
      questionText: "Question",
      jobId: toJobId("job-1"),
      createdAt: 123,
    });

    expect(JSON.parse(window.localStorage.getItem("interview_pending_answer_job_AP-123") ?? "null")).toEqual({
      clientRequestId: "client-1",
      turnId: "turn-1",
      answerText: "Answer",
      questionText: "Question",
      jobId: "job-1",
      createdAt: 123,
    });
    expect(window.localStorage.getItem("unrelated_key")).toBe("keep");
    expect(removeItemSpy).toHaveBeenCalledWith("interview_answer_draft_OLD_turn-1");
    expect(removeItemSpy).toHaveBeenCalledWith("interview_pending_answer_job_OLD");
    expect(setItemSpy).toHaveBeenCalledTimes(2);
  });
});
