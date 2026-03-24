import { test, expect } from "@playwright/test";
import { readRuntimeState } from "../../helpers/state.js";

test("manual PIN resume reaches interview entry without sessionStorage handoff", async ({ browser, request }) => {
  const { seededCouponCode } = readRuntimeState();

  const couponResponse = await request.post("/api/coupon/validate", {
    data: { code: seededCouponCode },
  });
  expect(couponResponse.ok()).toBeTruthy();
  const couponData = await couponResponse.json();

  const sessionCode = couponData.session_code as string;
  const pin = couponData.pin as string;
  expect(sessionCode).toBeTruthy();
  expect(pin).toBeTruthy();

  const context = await browser.newContext({ baseURL: "http://127.0.0.1:3000" });
  const page = await context.newPage();

  await page.goto(`/session/${sessionCode}?lang=en`);
  await expect(page.getByLabel("PIN")).toBeVisible();
  await expect(page.getByRole("button", { name: "Resume session" })).toBeVisible();

  await page.getByLabel("PIN").fill(pin);
  await page.getByRole("button", { name: "Resume session" }).click();

  await expect(page.getByRole("heading", { name: "Your session is ready" })).toBeVisible();
  await page.getByRole("button", { name: "Begin interview" }).click();
  await page.waitForURL(/\/interview\/[^?]+\?lang=en$/);
  await expect(page.getByRole("button", { name: "I understand" })).toBeVisible();

  await context.close();
});
