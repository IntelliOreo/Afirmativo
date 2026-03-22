import { NextRequest, NextResponse } from "next/server";

import { backendBaseURL } from "@/lib/env";

const hopByHopHeaders = new Set([
  "connection",
  "content-length",
  "host",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
]);

function buildBackendURL(base: string, request: NextRequest, path: string[]): string {
  const suffix = path.length > 0 ? `/${path.join("/")}` : "";
  return `${base}/api${suffix}${request.nextUrl.search}`;
}

function buildForwardHeaders(request: NextRequest): Headers {
  const headers = new Headers();

  for (const [key, value] of request.headers.entries()) {
    if (hopByHopHeaders.has(key.toLowerCase())) {
      continue;
    }
    headers.set(key, value);
  }

  headers.set("x-forwarded-host", request.headers.get("host") ?? "");
  headers.set("x-forwarded-proto", request.nextUrl.protocol.replace(":", ""));

  return headers;
}

async function proxy(request: NextRequest, path: string[]): Promise<Response> {
  const base = backendBaseURL();
  if (!base) {
    return NextResponse.json(
      {
        error: "Local API proxy target is not configured",
        code: "API_PROXY_TARGET_MISSING",
      },
      { status: 500 },
    );
  }

  const init: RequestInit = {
    method: request.method,
    headers: buildForwardHeaders(request),
    cache: "no-store",
    redirect: "manual",
  };

  if (request.method !== "GET" && request.method !== "HEAD") {
    init.body = await request.arrayBuffer();
  }

  const backendResponse = await fetch(buildBackendURL(base, request, path), init);
  const responseHeaders = new Headers(backendResponse.headers);

  return new Response(backendResponse.body, {
    status: backendResponse.status,
    statusText: backendResponse.statusText,
    headers: responseHeaders,
  });
}

type RouteContext = {
  params: Promise<{
    path: string[];
  }>;
};

export async function GET(request: NextRequest, context: RouteContext): Promise<Response> {
  const { path } = await context.params;
  return proxy(request, path);
}

export async function POST(request: NextRequest, context: RouteContext): Promise<Response> {
  const { path } = await context.params;
  return proxy(request, path);
}

export async function PUT(request: NextRequest, context: RouteContext): Promise<Response> {
  const { path } = await context.params;
  return proxy(request, path);
}

export async function PATCH(request: NextRequest, context: RouteContext): Promise<Response> {
  const { path } = await context.params;
  return proxy(request, path);
}

export async function DELETE(request: NextRequest, context: RouteContext): Promise<Response> {
  const { path } = await context.params;
  return proxy(request, path);
}

export async function OPTIONS(request: NextRequest, context: RouteContext): Promise<Response> {
  const { path } = await context.params;
  return proxy(request, path);
}

export async function HEAD(request: NextRequest, context: RouteContext): Promise<Response> {
  const { path } = await context.params;
  return proxy(request, path);
}
