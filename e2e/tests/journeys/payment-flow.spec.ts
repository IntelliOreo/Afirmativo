import { test, expect } from "@playwright/test";
import { signStripeWebhook } from "../../helpers/stripe-webhook.js";

const webhookSecret = "whsec_e2e_test_secret";

test("direct payment completes through webhook handoff and reaches interview entry", async ({ page, request }) => {
  await page.goto("/pay?lang=en");
  await page.getByRole("button", { name: "Pay $4.99 for 1 session" }).click();

  await page.waitForURL((url) =>
    url.pathname === "/pay/success" && (url.searchParams.get("session_id") ?? "").startsWith("cs_test_"),
  );
  await expect(page.getByTestId("payment-pending")).toBeVisible();

  const checkoutSessionID = new URL(page.url()).searchParams.get("session_id");
  expect(checkoutSessionID).toBeTruthy();

  const checkoutResponse = await request.get(`http://127.0.0.1:12111/__mock/checkout-sessions/${checkoutSessionID}`);
  expect(checkoutResponse.ok()).toBeTruthy();
  const checkout = await checkoutResponse.json();

  const payload = JSON.stringify({
    type: "checkout.session.completed",
    data: {
      object: {
        id: checkout.id,
        client_reference_id: checkout.client_reference_id,
        amount_total: checkout.amount_total,
        currency: checkout.currency,
        payment_status: "paid",
      },
    },
  });

  const webhookResponse = await request.post("http://127.0.0.1:8080/api/payment/webhook", {
    data: payload,
    headers: {
      "Content-Type": "application/json",
      "Stripe-Signature": signStripeWebhook(payload, webhookSecret),
    },
  });
  expect(webhookResponse.ok()).toBeTruthy();

  await page.waitForURL(/\/session\/[^?]+\?lang=en$/);
  await expect(page.getByRole("heading", { name: "Your session is ready" })).toBeVisible();
  await expect(page.getByTestId("session-pin")).toBeVisible();

  await page.getByRole("button", { name: "Begin interview" }).click();
  await page.waitForURL(/\/interview\/[^?]+\?lang=en$/);
  await expect(page.getByRole("button", { name: "I understand" })).toBeVisible();
});
