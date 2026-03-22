import { render, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import PaymentSuccessPage from "./page";

const replaceMock = vi.fn();
const apiMock = vi.fn();
const writePinMock = vi.fn();
const languageState = {
  lang: "en",
  initialized: true,
};
const searchParamsValues = new Map<string, string>([
  ["session_id", "cs_test_123"],
  ["lang", "en"],
]);

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    replace: replaceMock,
    push: vi.fn(),
  }),
  useSearchParams: () => ({
    get: (key: string) => searchParamsValues.get(key) ?? null,
  }),
}));

vi.mock("@/lib/api", () => ({
  api: (...args: unknown[]) => apiMock(...args),
}));

vi.mock("@/lib/storage/sessionPinStore", () => ({
  writePin: (...args: unknown[]) => writePinMock(...args),
}));

vi.mock("@/lib/useLanguage", () => ({
  useLanguage: () => ({
    lang: languageState.lang,
    setLang: vi.fn(),
    initialized: languageState.initialized,
  }),
}));

describe("PaymentSuccessPage", () => {
  beforeEach(() => {
    apiMock.mockReset();
    replaceMock.mockReset();
    writePinMock.mockReset();
    languageState.lang = "en";
    languageState.initialized = true;
    searchParamsValues.set("session_id", "cs_test_123");
    searchParamsValues.set("lang", "en");
  });

  it("stores the PIN and redirects when checkout status is ready", async () => {
    apiMock.mockResolvedValue({
      ok: true,
      status: 200,
      data: {
        status: "ready",
        session_code: "AP-1234-5678",
        pin: "482917",
      },
    });

    render(<PaymentSuccessPage />);

    await waitFor(() => {
      expect(writePinMock).toHaveBeenCalledWith("AP-1234-5678", "482917");
      expect(replaceMock).toHaveBeenCalledWith("/session/AP-1234-5678?lang=en");
    });
  });

  it("does not start polling before language initialization or restart polling when language changes", async () => {
    apiMock.mockResolvedValue({
      ok: true,
      status: 200,
      data: {
        status: "ready",
        session_code: "AP-1234-5678",
        pin: "482917",
      },
    });

    languageState.initialized = false;
    const { rerender } = render(<PaymentSuccessPage />);

    await waitFor(() => {
      expect(apiMock).toHaveBeenCalledTimes(0);
    });

    languageState.initialized = true;
    rerender(<PaymentSuccessPage />);

    await waitFor(() => {
      expect(apiMock).toHaveBeenCalledTimes(1);
    });

    languageState.lang = "es";
    rerender(<PaymentSuccessPage />);

    await waitFor(() => {
      expect(apiMock).toHaveBeenCalledTimes(1);
    });
  });
});
