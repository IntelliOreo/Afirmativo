import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import LandingPage from "./page";

const pushMock = vi.fn();
const verifySessionMock = vi.fn();

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
    lang: "en",
    setLang: vi.fn(),
  }),
}));

describe("LandingPage", () => {
  beforeEach(() => {
    pushMock.mockReset();
    verifySessionMock.mockReset();
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
});
