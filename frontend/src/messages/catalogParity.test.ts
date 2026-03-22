import { describe, expect, it } from "vitest";
import { LANDING_MESSAGES } from "./landingMessages";
import { PAY_MESSAGES } from "./payMessages";
import { SESSION_MESSAGES } from "./sessionMessages";
import { COMMON_MESSAGES } from "./commonMessages";
import { INTERVIEW_MESSAGES } from "@/app/interview/[code]/messages/interviewMessages";

function isObject(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

function collectKeys(value: unknown, prefix = ""): string[] {
  if (!isObject(value)) return [];

  return Object.entries(value).flatMap(([key, nested]) => {
    const nextPrefix = prefix ? `${prefix}.${key}` : key;
    if (!isObject(nested)) return [nextPrefix];
    return collectKeys(nested, nextPrefix);
  });
}

function expectCatalogParity(catalog: Record<"en" | "es", unknown>) {
  const enKeys = collectKeys(catalog.en).sort();
  const esKeys = collectKeys(catalog.es).sort();
  expect(esKeys).toEqual(enKeys);
}

describe("message catalogs", () => {
  it("keep translation keys aligned", () => {
    expectCatalogParity(COMMON_MESSAGES);
    expectCatalogParity(LANDING_MESSAGES);
    expectCatalogParity(SESSION_MESSAGES);
    expectCatalogParity(PAY_MESSAGES);
    expectCatalogParity(INTERVIEW_MESSAGES);
  });
});
