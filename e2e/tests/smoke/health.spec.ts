import { test, expect } from "@playwright/test";

test("health responds through the frontend proxy", async ({ request }) => {
  const response = await request.get("/api/health");
  expect(response.ok()).toBeTruthy();

  const body = await response.json();
  expect(body.health_scope).toBe("instance_local");
});
