import { notFound } from "next/navigation";

import { isDevEnv } from "@/lib/env";

import { AdminPageClient } from "./pageClient";

export default function AdminPage() {
  if (!isDevEnv()) {
    notFound();
  }

  return <AdminPageClient />;
}

