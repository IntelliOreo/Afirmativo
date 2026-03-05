import { NextRequest, NextResponse } from "next/server";
import { isAdminToolsEnabled } from "./src/lib/env";

export function middleware(request: NextRequest) {
  if (isAdminToolsEnabled()) {
    return NextResponse.next();
  }

  if (request.nextUrl.pathname.startsWith("/api/admin")) {
    return NextResponse.json({ error: "Not Found" }, { status: 404 });
  }

  return new NextResponse("Not Found", {
    status: 404,
    headers: { "content-type": "text/plain; charset=utf-8" },
  });
}

export const config = {
  matcher: ["/admin/:path*", "/api/admin/:path*"],
};
