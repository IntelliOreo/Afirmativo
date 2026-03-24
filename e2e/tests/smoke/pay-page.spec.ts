import { test, expect } from "@playwright/test";
import { expectNoPageErrors } from "../helpers.js";

test("pay page loads in English", async ({ page }) => {
  await page.goto("/pay?lang=en");
  await expect(page.getByRole("heading", { name: "Access" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Apply coupon" })).toBeVisible();
  await expectNoPageErrors(page);
});

test("pay page loads in Spanish", async ({ page }) => {
  await page.goto("/pay?lang=es");
  await expect(page.getByRole("heading", { name: "Acceso" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Aplicar cupon" })).toBeVisible();
  await expectNoPageErrors(page);
});
