import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { afterAll, beforeEach, describe, expect, it, vi } from "vitest";
import PaymentSuccessPage from "./page";

const apiMock = vi.fn();
const replaceMock = vi.fn();
const pushMock = vi.fn();
const writePinMock = vi.fn();
const setLangMock = vi.fn();
const useLanguageMock = vi.fn();
const originalLocation = window.location;

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    replace: replaceMock,
    push: pushMock,
  }),
  useSearchParams: () => ({
    get: (key: string) => {
      if (key === "lang") return "en";
      if (key === "session_id") return "cs_test_123";
      return null;
    },
  }),
}));

vi.mock("@/lib/api", () => ({
  api: (...args: unknown[]) => apiMock(...args),
}));

vi.mock("@/lib/useLanguage", () => ({
  useLanguage: (...args: unknown[]) => useLanguageMock(...args),
}));

vi.mock("@/lib/storage/sessionPinStore", () => ({
  writePin: (...args: unknown[]) => writePinMock(...args),
}));

describe("PaymentSuccessPage", () => {
  beforeEach(() => {
    apiMock.mockReset();
    replaceMock.mockReset();
    pushMock.mockReset();
    writePinMock.mockReset();
    setLangMock.mockReset();
    useLanguageMock.mockReset();
    useLanguageMock.mockReturnValue({
      lang: "en",
      setLang: setLangMock,
      initialized: true,
    });
    Object.defineProperty(window, "location", {
      configurable: true,
      value: { href: "http://localhost/pay/success?session_id=cs_test_123", origin: "http://localhost" } as Location,
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

  it("redirects direct-session checkouts into the session page", async () => {
    apiMock.mockResolvedValue({
      ok: true,
      status: 200,
      data: {
        status: "ready",
        product_type: "direct_session",
        session_code: "AP-1234-5678",
        pin: "482917",
      },
    });

    render(<PaymentSuccessPage />);

    await waitFor(() => {
      expect(writePinMock).toHaveBeenCalledWith("AP-1234-5678", "482917");
    });
    expect(replaceMock).toHaveBeenCalledWith("/session/AP-1234-5678?lang=en");
  });

  it("renders coupon-pack ready state and opens a mailto handoff", async () => {
    apiMock.mockResolvedValue({
      ok: true,
      status: 200,
      data: {
        status: "ready",
        product_type: "coupon_pack_10",
        coupon_code: "PACK10-ABCD2345",
        coupon_max_uses: 10,
        coupon_current_uses: 0,
      },
    });

    render(<PaymentSuccessPage />);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Your coupon is ready" })).toBeInTheDocument();
    });
    expect(screen.getByText("PACK10-ABCD2345")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Email my coupon info" }));

    const mailto = parseMailtoHref(window.location.href);
    expect(mailto.subject).toBe("asilo-afirmativo: coupon info");
    expect(mailto.body).toBe([
      "Coupon: PACK10-ABCD2345",
      "This coupon can be redeemed up to 10 times. This was redemption 0 of 10.",
      "Link: http://localhost/pay?coupon=PACK10-ABCD2345&lang=en",
    ].join("\n"));
  });

  it("treats omitted coupon_current_uses as zero for coupon-pack ready responses", async () => {
    apiMock.mockResolvedValue({
      ok: true,
      status: 200,
      data: {
        status: "ready",
        product_type: "coupon_pack_10",
        coupon_code: "PACK10-ABCD2345",
        coupon_max_uses: 10,
      },
    });

    render(<PaymentSuccessPage />);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Your coupon is ready" })).toBeInTheDocument();
    });
    expect(screen.getByText("This coupon can be redeemed up to 10 times. This was redemption 0 of 10.")).toBeInTheDocument();
  });
});
