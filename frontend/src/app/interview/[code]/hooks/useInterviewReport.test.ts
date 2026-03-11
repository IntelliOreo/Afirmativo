import { renderHook, waitFor } from "@testing-library/react";
import { act } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useInterviewReport } from "./useInterviewReport";

const apiMock = vi.fn();

vi.mock("@/lib/api", () => ({
  api: (...args: unknown[]) => apiMock(...args),
}));

describe("useInterviewReport", () => {
  beforeEach(() => {
    apiMock.mockReset();
  });

  it("leaves report state idle when resume sees REPORT_NOT_STARTED", async () => {
    apiMock.mockResolvedValue({
      ok: false,
      status: 404,
      data: { error: "Not started", code: "REPORT_NOT_STARTED" },
    });

    const { result } = renderHook(() => useInterviewReport("AP-123"));

    await act(async () => {
      await result.current.resumeReport();
    });

    expect(result.current.reportStatus).toBe("idle");
    expect(result.current.report).toBeNull();
  });

  it("enters generating state when POST /generate accepts the job", async () => {
    apiMock.mockResolvedValue({
      ok: true,
      status: 202,
      data: { status: "generating" },
    });

    const { result } = renderHook(() => useInterviewReport("AP-123"));

    await act(async () => {
      await result.current.loadReport();
    });

    expect(result.current.reportStatus).toBe("generating");
  });

  it("stores canonical report fields when the API returns them", async () => {
    apiMock.mockResolvedValue({
      ok: true,
      status: 200,
      data: {
        session_code: "AP-123",
        status: "ready",
        content_en: "English content",
        content_es: "Contenido",
        areas_of_clarity: ["Strong chronology"],
        areas_of_clarity_es: ["Cronologia solida"],
        areas_to_develop_further: ["More dates"],
        areas_to_develop_further_es: ["Mas fechas"],
        recommendation: "Practice timeline details",
        recommendation_es: "Practique los detalles de la cronologia",
        question_count: 12,
        duration_minutes: 25,
      },
    });

    const { result } = renderHook(() => useInterviewReport("AP-123"));

    await act(async () => {
      await result.current.resumeReport();
    });

    expect(result.current.reportStatus).toBe("ready");
    expect(result.current.report?.areasOfClarity).toEqual(["Strong chronology"]);
    expect(result.current.report?.areasOfClarityEs).toEqual(["Cronologia solida"]);
  });

  it("surfaces API failures as report errors", async () => {
    apiMock.mockResolvedValue({
      ok: false,
      status: 500,
      data: { error: "Report failed", code: "GENERATION_FAILED" },
    });

    const { result } = renderHook(() => useInterviewReport("AP-123"));

    await act(async () => {
      await result.current.resumeReport();
    });

    await waitFor(() => {
      expect(result.current.reportStatus).toBe("error");
    });
    expect(result.current.reportError).toBe("Report failed");
  });
});
