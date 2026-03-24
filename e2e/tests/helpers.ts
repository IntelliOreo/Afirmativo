import { expect, Page } from "@playwright/test";

export async function expectNoPageErrors(page: Page): Promise<void> {
  const pageErrors: Error[] = [];
  page.on("pageerror", (error) => {
    pageErrors.push(error);
  });

  await page.waitForLoadState("networkidle");
  expect(pageErrors, pageErrors.map((error) => error.message).join("\n")).toEqual([]);
}
