import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import LandingPage from "./page";

const pushMock = vi.fn();
const verifySessionMock = vi.fn();
const setLangMock = vi.fn();
let mockLang: "en" | "es" = "en";

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: pushMock,
  }),
}));

vi.mock("next/link", () => ({
  default: ({ href, children }: { href: string; children: React.ReactNode }) => (
    <a href={href}>{children}</a>
  ),
}));

vi.mock("@/lib/env", () => ({
  isAdminToolsEnabled: () => false,
}));

vi.mock("@/lib/sessionService", () => ({
  verifySession: (...args: unknown[]) => verifySessionMock(...args),
}));

vi.mock("@/lib/useLanguage", () => ({
  useLanguage: () => ({
    lang: mockLang,
    setLang: setLangMock,
  }),
}));

describe("LandingPage", () => {
  beforeEach(() => {
    pushMock.mockReset();
    verifySessionMock.mockReset();
    setLangMock.mockReset();
    mockLang = "en";
  });

  it("sends a verified returning user straight to the interview page", async () => {
    verifySessionMock.mockResolvedValue({ ok: true });

    render(<LandingPage />);

    fireEvent.change(screen.getByLabelText("Session code"), {
      target: { value: "AP-7K9X-M2NF" },
    });
    fireEvent.change(screen.getByLabelText("PIN"), {
      target: { value: "482917" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Resume session" }));

    await waitFor(() => {
      expect(pushMock).toHaveBeenCalledWith("/interview/AP-7K9X-M2NF?lang=en");
    });
  });

  it("shows the invalid PIN message in the active language only", async () => {
    verifySessionMock.mockResolvedValue({ ok: false, reason: "invalid_pin" });

    render(<LandingPage />);

    fireEvent.change(screen.getByLabelText("Session code"), {
      target: { value: "AP-7K9X-M2NF" },
    });
    fireEvent.change(screen.getByLabelText("PIN"), {
      target: { value: "000000" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Resume session" }));

    await waitFor(() => {
      expect(screen.getByText("Incorrect PIN. Please try again.")).toBeInTheDocument();
    });
  });

  it("renders both testimonials on the landing page without the removed review banner", () => {
    render(<LandingPage />);

    expect(screen.queryByText("Based on real reviews")).not.toBeInTheDocument();
    expect(screen.getByText("Read before you start and agree to the terms")).toBeInTheDocument();
    expect(
      screen.getByText(
        "“Practicar esto de verdad ha hecho una gran diferencia. Explicar lo que pasó ahora me sale mucho más claro y natural. Antes era un punto débil, pero ha mejorado bastante.”",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "“Gracias, ya llevo más de diez sesiones practicando y, de verdad, me ha ayudado muchísimo a bajar la ansiedad.”",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("G.")).toBeInTheDocument();
    expect(screen.getByText("J.")).toBeInTheDocument();
  });

  it("keeps the testimonial quotes verbatim in Spanish mode", () => {
    mockLang = "es";

    render(<LandingPage />);

    expect(screen.queryByText("Basado en resenas reales")).not.toBeInTheDocument();
    expect(screen.getByText("Lea antes de comenzar y acepte los terminos")).toBeInTheDocument();
    expect(
      screen.getByText(
        "“Practicar esto de verdad ha hecho una gran diferencia. Explicar lo que pasó ahora me sale mucho más claro y natural. Antes era un punto débil, pero ha mejorado bastante.”",
      ),
    ).toBeInTheDocument();
  });
});
