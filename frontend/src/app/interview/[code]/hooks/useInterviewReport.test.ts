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

  it("keeps the hook in generating state on 202", async () => {
    apiMock.mockResolvedValue({
      ok: true,
      status: 202,
      data: null,
    });

    const { result } = renderHook(() => useInterviewReport("AP-123"));

    await act(async () => {
      await result.current.loadReport();
    });

    expect(result.current.reportStatus).toBe("generating");
    expect(result.current.report).toBeNull();
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
        areas_to_develop_further: ["More dates"],
        recommendation: "Practice timeline details",
        question_count: 12,
        duration_minutes: 25,
      },
    });

    const { result } = renderHook(() => useInterviewReport("AP-123"));

    await act(async () => {
      await result.current.loadReport();
    });

    expect(result.current.reportStatus).toBe("ready");
    expect(result.current.report?.areasOfClarity).toEqual(["Strong chronology"]);
    expect(result.current.report?.areasToDevelopFurther).toEqual(["More dates"]);
  });

  it("normalizes missing canonical arrays to empty arrays", async () => {
    apiMock.mockResolvedValue({
      ok: true,
      status: 200,
      data: {
        session_code: "AP-123",
        status: "ready",
        content_en: "English content",
        content_es: "Contenido",
        recommendation: "Practice timeline details",
        question_count: 12,
        duration_minutes: 25,
      },
    });

    const { result } = renderHook(() => useInterviewReport("AP-123"));

    await act(async () => {
      await result.current.loadReport();
    });

    expect(result.current.reportStatus).toBe("ready");
    expect(result.current.report?.areasOfClarity).toEqual([]);
    expect(result.current.report?.areasToDevelopFurther).toEqual([]);
  });

  it("does not backfill legacy strengths and weaknesses into canonical arrays", async () => {
    apiMock.mockResolvedValue({
      ok: true,
      status: 200,
      data: {
        session_code: "AP-123",
        status: "ready",
        content_en: "English content",
        content_es: "Contenido",
        strengths: ["Legacy strength"],
        weaknesses: ["Legacy weakness"],
        recommendation: "Practice timeline details",
        question_count: 12,
        duration_minutes: 25,
      },
    });

    const { result } = renderHook(() => useInterviewReport("AP-123"));

    await act(async () => {
      await result.current.loadReport();
    });

    expect(result.current.reportStatus).toBe("ready");
    expect(result.current.report?.areasOfClarity).toEqual([]);
    expect(result.current.report?.areasToDevelopFurther).toEqual([]);
  });

  it("surfaces API failures as report errors", async () => {
    apiMock.mockResolvedValue({
      ok: false,
      status: 500,
      data: { error: "Report failed" },
    });

    const { result } = renderHook(() => useInterviewReport("AP-123"));

    await act(async () => {
      await result.current.loadReport();
    });

    await waitFor(() => {
      expect(result.current.reportStatus).toBe("error");
    });
    expect(result.current.reportError).toBe("Report failed");
  });
});
