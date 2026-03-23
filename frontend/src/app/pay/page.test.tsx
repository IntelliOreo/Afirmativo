import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { afterAll, beforeEach, describe, expect, it, vi } from "vitest";
import PayPage from "./page";

const apiMock = vi.fn();
const pushMock = vi.fn();
const writePinMock = vi.fn();
const writeCouponRevealMock = vi.fn();
const originalLocation = window.location;

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: pushMock,
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

vi.mock("@/lib/storage/sessionPinStore", () => ({
  writePin: (...args: unknown[]) => writePinMock(...args),
  writeCouponReveal: (...args: unknown[]) => writeCouponRevealMock(...args),
}));

describe("PayPage", () => {
  beforeEach(() => {
    apiMock.mockReset();
    pushMock.mockReset();
    writePinMock.mockReset();
    writeCouponRevealMock.mockReset();
    Object.defineProperty(window, "location", {
      configurable: true,
      value: { href: "http://localhost/pay" } as Location,
    });
  });

  afterAll(() => {
    Object.defineProperty(window, "location", {
      configurable: true,
      value: originalLocation,
    });
  });

  it("redirects to the Stripe checkout URL with the selected language", async () => {
    apiMock.mockResolvedValue({
      ok: true,
      status: 200,
      data: { url: "https://checkout.stripe.com/c/pay/cs_test_123" },
    });

    render(<PayPage />);

    fireEvent.click(screen.getByRole("button", { name: "Pay by card" }));

    await waitFor(() => {
      expect(apiMock).toHaveBeenCalledWith("/api/payment/checkout", {
        method: "POST",
        body: { lang: "en" },
      });
    });

    await waitFor(() => {
      expect(window.location.href).toBe("https://checkout.stripe.com/c/pay/cs_test_123");
    });
  });

  it("stores coupon reveal data and routes to the session page after coupon redemption", async () => {
    apiMock.mockResolvedValue({
      ok: true,
      status: 200,
      data: {
        valid: true,
        session_code: "AP-1234-5678",
        pin: "482917",
        coupon: {
          code: "BETA-0001",
          max_uses: 5,
          current_uses: 2,
        },
      },
    });

    render(<PayPage />);

    fireEvent.change(screen.getByLabelText("Coupon"), {
      target: { value: "BETA-0001" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Apply coupon" }));

    await waitFor(() => {
      expect(apiMock).toHaveBeenCalledWith("/api/coupon/validate", {
        method: "POST",
        body: { code: "BETA-0001" },
      });
    });

    expect(writePinMock).toHaveBeenCalledWith("AP-1234-5678", "482917");
    expect(writeCouponRevealMock).toHaveBeenCalledWith("AP-1234-5678", {
      code: "BETA-0001",
      maxUses: 5,
      currentUses: 2,
    });
    expect(pushMock).toHaveBeenCalledWith("/session/AP-1234-5678?lang=en");
  });

  it("shows the generic checkout failure for resolved non-ok responses", async () => {
    apiMock.mockResolvedValue({
      ok: false,
      status: 500,
      data: { code: "PAYMENT_CHECKOUT_FAILED" },
    });

    render(<PayPage />);

    fireEvent.click(screen.getByRole("button", { name: "Pay by card" }));

    expect(await screen.findByText("Could not start checkout. Please try again.")).toBeInTheDocument();
    expect(screen.queryByText("Card payment is not available yet. Please use a coupon to continue.")).not.toBeInTheDocument();
  });

  it("shows the network checkout failure when the request rejects", async () => {
    apiMock.mockRejectedValue(new Error("network down"));

    render(<PayPage />);

    fireEvent.click(screen.getByRole("button", { name: "Pay by card" }));

    expect(await screen.findByText("Connection error while starting checkout. Please try again.")).toBeInTheDocument();
  });
});
