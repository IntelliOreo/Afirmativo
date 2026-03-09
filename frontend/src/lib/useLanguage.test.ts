import { renderHook, waitFor } from "@testing-library/react";
import { act } from "react";
import { describe, expect, it } from "vitest";
import { useLanguage } from "./useLanguage";
import {
  readInterviewLang,
  readUiLang,
  writeInterviewLang,
  writeUiLang,
} from "./storage/languageStore";

describe("useLanguage", () => {
  it("prefers the query parameter over stored values and persists it", async () => {
    writeUiLang("es");
    writeInterviewLang("AP-123", "es");

    const { result } = renderHook(() => useLanguage({
      requestedLang: "en",
      sessionCode: "AP-123",
    }));

    await waitFor(() => {
      expect(result.current.initialized).toBe(true);
    });
    expect(result.current.lang).toBe("en");
    expect(readUiLang()).toBe("en");
    expect(readInterviewLang("AP-123")).toBe("en");
  });

  it("falls back to interview-scoped language before the ui language", async () => {
    writeUiLang("en");
    writeInterviewLang("AP-123", "es");

    const { result } = renderHook(() => useLanguage({
      requestedLang: null,
      sessionCode: "AP-123",
    }));

    await waitFor(() => {
      expect(result.current.initialized).toBe(true);
    });
    expect(result.current.lang).toBe("es");
  });

  it("writes both ui and interview language when session-scoped language changes", async () => {
    const { result } = renderHook(() => useLanguage({
      requestedLang: null,
      sessionCode: "AP-123",
    }));

    await waitFor(() => {
      expect(result.current.initialized).toBe(true);
    });

    act(() => {
      result.current.setLang("en");
    });

    await waitFor(() => {
      expect(readUiLang()).toBe("en");
    });
    expect(readInterviewLang("AP-123")).toBe("en");
  });
});
