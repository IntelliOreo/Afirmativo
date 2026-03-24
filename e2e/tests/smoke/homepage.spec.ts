import { test, expect } from "@playwright/test";
import { expectNoPageErrors } from "../helpers.js";

test("homepage loads", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("link", { name: /Affirmative Interview Simulator|Simulador de Entrevista Afirmativa/ })).toBeVisible();
  await expect(page.getByText(/Read before you start and agree to the terms|Lea antes de comenzar y acepte los terminos/)).toBeVisible();
  await expectNoPageErrors(page);
});
