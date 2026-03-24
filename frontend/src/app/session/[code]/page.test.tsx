import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { afterAll, beforeEach, describe, expect, it, vi } from "vitest";
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
const originalLocation = window.location;

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
    useSearchParamsMock.mockReset();
    readAndConsumePinMock.mockReset();
    verifySessionMock.mockReset();
    checkSessionAccessMock.mockReset();
    writeInterviewLangMock.mockReset();
    setLangMock.mockReset();
    useLanguageMock.mockReset();
    useSearchParamsMock.mockReturnValue({ get: () => "en" });
    useLanguageMock.mockReturnValue({
      lang: "en",
      setLang: setLangMock,
      initialized: true,
    });
    Object.defineProperty(window, "location", {
      configurable: true,
      value: { href: "http://localhost/session/AP-123", origin: "http://localhost" } as Location,
    });
  });

  afterAll(() => {
    Object.defineProperty(window, "location", {
      configurable: true,
      value: originalLocation,
    });
  });

  function parseMailtoHref(href: string) {
    const [scheme, query = ""] = href.split("?");
    expect(scheme).toBe("mailto:");
    const params = new URLSearchParams(query);
    return {
      subject: params.get("subject"),
      body: params.get("body"),
    };
  }

  it("shows the ready state when a stored PIN verifies a not-started session", async () => {
    readAndConsumePinMock.mockReturnValue("482917");
    verifySessionMock.mockResolvedValue({
      ok: true,
      coupon: {
        code: "BETA-0001",
        maxUses: 5,
        currentUses: 2,
      },
    });

    render(<SessionPage />);

    await waitFor(() => {
      expect(screen.getByText("Your session is ready")).toBeInTheDocument();
    });
    expect(screen.getByText("This information will not be shown again automatically. Please copy it or save it somewhere safe before you begin.")).toBeInTheDocument();
    expect(screen.getByText("BETA-0001")).toBeInTheDocument();
    expect(screen.getByText("This coupon can be redeemed up to 5 times. This was redemption 2 of 5.")).toBeInTheDocument();
    expect(screen.getByText("Below is your current session information.")).toBeInTheDocument();
    expect(screen.getByText("482917")).toBeInTheDocument();
    expect(replaceMock).not.toHaveBeenCalled();
  });

  it("opens a mailto link with the full reveal payload in English", async () => {
    readAndConsumePinMock.mockReturnValue("482917");
    verifySessionMock.mockResolvedValue({
      ok: true,
      coupon: {
        code: "BETA-0001",
        maxUses: 5,
        currentUses: 2,
      },
    });

    render(<SessionPage />);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Email my session/coupon info" })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Email my session/coupon info" }));

    const mailto = parseMailtoHref(window.location.href);
    expect(mailto.subject).toBe("asilo-afirmativo: session/coupon info");
    expect(mailto.body).toBe([
      "Coupon: BETA-0001",
      "This coupon can be redeemed up to 5 times. This was redemption 2 of 5.",
      "",
      "Below is your current session information.",
      "Session code: AP-123",
      "PIN: 482917",
      "Link: http://localhost/session/AP-123",
    ].join("\n"));
  });

  it("opens a localized mailto link in Spanish", async () => {
    useSearchParamsMock.mockReturnValue({ get: () => "es" });
    useLanguageMock.mockReturnValue({
      lang: "es",
      setLang: setLangMock,
      initialized: true,
    });
    readAndConsumePinMock.mockReturnValue("482917");
    verifySessionMock.mockResolvedValue({ ok: true });

    render(<SessionPage />);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Enviarme por correo la informacion de mi sesion/cupon" })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Enviarme por correo la informacion de mi sesion/cupon" }));

    const mailto = parseMailtoHref(window.location.href);
    expect(mailto.subject).toBe("asilo-afirmativo: informacion de sesion/cupon");
    expect(mailto.body).toBe([
      "Codigo de sesion: AP-123",
      "PIN: 482917",
      "Enlace: http://localhost/session/AP-123",
    ].join("\n"));
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
