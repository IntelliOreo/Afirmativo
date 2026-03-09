"use client";

import Link from "next/link";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import type { Lang } from "@/lib/language";

interface InterviewGuardStateProps {
  lang: Lang;
  code: string;
}

export function InterviewGuardState({ lang, code }: InterviewGuardStateProps) {
  return (
    <Card>
      <h1 className="text-2xl font-bold text-primary-dark mb-4">
        {lang === "es" ? "Sesión no encontrada" : "Session not found"}
      </h1>
      <p className="text-primary-darkest mb-6">
        {lang === "es"
          ? "No se encontró una sesión activa en este navegador. Si tiene un código de sesión y PIN, puede recuperar su acceso."
          : "No active session found in this browser. If you have a session code and PIN, you can recover your access."}
      </p>
      <Link href={`/session/${code}`}>
        <Button fullWidth>
          {lang === "es" ? "Recuperar sesión" : "Recover session"}
        </Button>
      </Link>
    </Card>
  );
}
