import { render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SessionPage from "./page";

const pushMock = vi.fn();
const replaceMock = vi.fn();
const useParamsMock = vi.fn(() => ({ code: "AP-123" }));
const useSearchParamsMock = vi.fn(() => ({ get: () => "en" }));
const readAndConsumePinMock = vi.fn();
const verifySessionMock = vi.fn();
const checkSessionAccessMock = vi.fn();
const writeInterviewLangMock = vi.fn();
const setLangMock = vi.fn();
const useLanguageMock = vi.fn();

vi.mock("next/navigation", () => ({
  useParams: () => useParamsMock(),
  useRouter: () => ({
    push: pushMock,
    replace: replaceMock,
  }),
  useSearchParams: () => useSearchParamsMock(),
}));

vi.mock("@/lib/storage/sessionPinStore", () => ({
  readAndConsumePin: (...args: unknown[]) => readAndConsumePinMock(...args),
}));

vi.mock("@/lib/storage/languageStore", () => ({
  writeInterviewLang: (...args: unknown[]) => writeInterviewLangMock(...args),
}));

vi.mock("@/lib/sessionService", () => ({
  verifySession: (...args: unknown[]) => verifySessionMock(...args),
  checkSessionAccess: (...args: unknown[]) => checkSessionAccessMock(...args),
}));

vi.mock("@/lib/useLanguage", () => ({
  useLanguage: (...args: unknown[]) => useLanguageMock(...args),
}));

describe("SessionPage", () => {
  beforeEach(() => {
    pushMock.mockReset();
    replaceMock.mockReset();
    readAndConsumePinMock.mockReset();
    verifySessionMock.mockReset();
    checkSessionAccessMock.mockReset();
    writeInterviewLangMock.mockReset();
    setLangMock.mockReset();
    useLanguageMock.mockReset();
    useLanguageMock.mockReturnValue({
      lang: "en",
      setLang: setLangMock,
      initialized: true,
    });
  });

  it("shows the ready state when a stored PIN verifies a not-started session", async () => {
    readAndConsumePinMock.mockReturnValue("482917");
    verifySessionMock.mockResolvedValue({ ok: true });

    render(<SessionPage />);

    await waitFor(() => {
      expect(screen.getByText("Your session is ready")).toBeInTheDocument();
    });
    expect(screen.getByText("482917")).toBeInTheDocument();
    expect(replaceMock).not.toHaveBeenCalled();
  });

  it("goes straight to the interview when a valid cookie is already present", async () => {
    readAndConsumePinMock.mockReturnValue(null);
    checkSessionAccessMock.mockResolvedValue({ ok: true });

    render(<SessionPage />);

    await waitFor(() => {
      expect(replaceMock).toHaveBeenCalledWith("/interview/AP-123?lang=en");
    });
    expect(screen.queryByText("Resume session")).not.toBeInTheDocument();
  });

  it("shows verification when there is no stored PIN and cookie access is unauthorized", async () => {
    readAndConsumePinMock.mockReturnValue(null);
    checkSessionAccessMock.mockResolvedValue({ ok: false, reason: "unauthorized" });

    render(<SessionPage />);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Resume session" })).toBeInTheDocument();
    });
  });

  it("consumes the handoff PIN only once across rerenders", async () => {
    readAndConsumePinMock.mockReturnValue("482917");
    verifySessionMock.mockReturnValue(new Promise(() => {}));

    const { rerender, unmount } = render(<SessionPage />);
    rerender(<SessionPage />);

    await waitFor(() => {
      expect(readAndConsumePinMock).toHaveBeenCalledTimes(1);
      expect(verifySessionMock).toHaveBeenCalledTimes(1);
    });
    expect(checkSessionAccessMock).not.toHaveBeenCalled();
    unmount();
  });

  it("does not fall back to cookie access while stored PIN verification is pending", async () => {
    readAndConsumePinMock.mockReturnValue("482917");
    verifySessionMock.mockReturnValue(new Promise(() => {}));

    const { rerender, unmount } = render(<SessionPage />);
    rerender(<SessionPage />);

    await waitFor(() => {
      expect(verifySessionMock).toHaveBeenCalledTimes(1);
    });
    expect(checkSessionAccessMock).not.toHaveBeenCalled();
    expect(screen.queryByRole("heading", { name: "Resume session" })).not.toBeInTheDocument();
    unmount();
  });

  it("waits for language initialization before bootstrapping session access", async () => {
    useLanguageMock.mockReturnValue({
      lang: "en",
      setLang: setLangMock,
      initialized: false,
    });
    readAndConsumePinMock.mockReturnValue("482917");
    verifySessionMock.mockResolvedValue({ ok: true });

    const { rerender } = render(<SessionPage />);

    expect(readAndConsumePinMock).not.toHaveBeenCalled();
    expect(checkSessionAccessMock).not.toHaveBeenCalled();

    useLanguageMock.mockReturnValue({
      lang: "en",
      setLang: setLangMock,
      initialized: true,
    });

    rerender(<SessionPage />);

    await waitFor(() => {
      expect(readAndConsumePinMock).toHaveBeenCalledTimes(1);
    });
    await waitFor(() => {
      expect(screen.getByText("Your session is ready")).toBeInTheDocument();
    });
  });
});
