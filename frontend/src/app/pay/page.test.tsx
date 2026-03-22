import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import PayPage from "./page";

const apiMock = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: vi.fn(),
  }),
  useSearchParams: () => ({
    get: () => "en",
  }),
}));

vi.mock("@/lib/api", () => ({
  api: (...args: unknown[]) => apiMock(...args),
}));

vi.mock("@/lib/useLanguage", () => ({
  useLanguage: () => ({
    lang: "en",
    setLang: vi.fn(),
  }),
}));

describe("PayPage", () => {
  beforeEach(() => {
    apiMock.mockReset();
  });

  it("starts checkout with the selected language", async () => {
    apiMock.mockResolvedValue({
      ok: false,
      status: 500,
      data: {},
    });

    render(<PayPage />);

    fireEvent.click(screen.getByRole("button", { name: "Pay by card" }));

    await waitFor(() => {
      expect(apiMock).toHaveBeenCalledWith("/api/payment/checkout", {
        method: "POST",
        body: { lang: "en" },
      });
    });
  });
});
