import { test, expect } from "@playwright/test";
import { readRuntimeState } from "../../helpers/state.js";

test("coupon redemption hands off to session and reaches interview entry", async ({ page, context }) => {
  const { seededCouponCode } = readRuntimeState();

  await page.goto("/pay?lang=en");
  await page.getByLabel("Coupon").fill(seededCouponCode);
  await page.getByRole("button", { name: "Apply coupon" }).click();

  await page.waitForURL(/\/session\/[^?]+\?lang=en$/);
  await expect(page.getByRole("heading", { name: "Your session is ready" })).toBeVisible();

  const sessionCode = page.url().match(/\/session\/([^?]+)/)?.[1];
  expect(sessionCode).toBeTruthy();

  const storedPin = await page.evaluate((code) => sessionStorage.getItem(`pin_${code}`), sessionCode);
  expect(storedPin).toBeNull();

  const cookies = await context.cookies();
  expect(cookies.some((cookie) => cookie.name === "afirmativo_auth")).toBeTruthy();

  await page.getByRole("button", { name: "Begin interview" }).click();
  await page.waitForURL(/\/interview\/[^?]+\?lang=en$/);
  await expect(page.getByRole("button", { name: "I understand" })).toBeVisible();
});
