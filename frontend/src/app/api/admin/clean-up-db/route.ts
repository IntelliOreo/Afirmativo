import { NextRequest, NextResponse } from "next/server";

import { isAdminToolsEnabled } from "@/lib/env";

type CleanUpRequest = {
  hours?: number;
};

function backendBaseURL(): string {
  const raw = process.env.API_URL ?? process.env.NEXT_PUBLIC_API_URL ?? "";
  return raw.replace(/\/+$/, "");
}

export async function POST(request: NextRequest) {
  if (!isAdminToolsEnabled()) {
    return NextResponse.json(
      { error: "Admin endpoint disabled outside dev env", code: "ADMIN_DISABLED" },
      { status: 404 },
    );
  }

  const apiURL = backendBaseURL();
  if (!apiURL) {
    return NextResponse.json(
      { error: "Backend API URL is not configured", code: "BACKEND_URL_MISSING" },
      { status: 500 },
    );
  }

  let payload: CleanUpRequest = {};
  const rawBody = await request.text();
  if (rawBody.trim() !== "") {
    let parsed: unknown;
    try {
      parsed = JSON.parse(rawBody);
    } catch {
      return NextResponse.json(
        { error: "Invalid JSON body", code: "BAD_REQUEST" },
        { status: 400 },
      );
    }

    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
      return NextResponse.json(
        { error: "Body must be a JSON object", code: "BAD_REQUEST" },
        { status: 400 },
      );
    }

    const candidate = parsed as CleanUpRequest;
    if (
      Object.prototype.hasOwnProperty.call(candidate, "hours")
      && typeof candidate.hours !== "number"
    ) {
      return NextResponse.json(
        { error: "hours must be a number", code: "BAD_REQUEST" },
        { status: 400 },
      );
    }
    payload = candidate;
  }

  const backendResponse = await fetch(`${apiURL}/api/admin/clean-up-db`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
    cache: "no-store",
  });

  const contentType = backendResponse.headers.get("content-type") ?? "";
  if (contentType.includes("application/json")) {
    const json = await backendResponse.json();
    return NextResponse.json(json, { status: backendResponse.status });
  }

  if (backendResponse.status === 404) {
    return NextResponse.json(
      {
        error: "Backend admin cleanup endpoint is disabled",
        code: "ADMIN_DISABLED",
      },
      { status: 404 },
    );
  }

  const text = await backendResponse.text();
  return NextResponse.json(
    {
      error: "Backend returned a non-JSON response",
      code: "BAD_BACKEND_RESPONSE",
      backend_status: backendResponse.status,
      details: text.slice(0, 300),
    },
    { status: 502 },
  );
}
