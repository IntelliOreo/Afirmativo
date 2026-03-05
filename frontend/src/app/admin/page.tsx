import { notFound } from "next/navigation";

import { isAdminToolsEnabled } from "@/lib/env";

import { AdminPageClient } from "./pageClient";

export default function AdminPage() {
  if (!isAdminToolsEnabled()) {
    notFound();
  }

  return <AdminPageClient />;
}
