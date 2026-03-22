"use client";

import Link from "next/link";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import type { Lang } from "@/lib/language";
import { getInterviewMessages } from "../messages/interviewMessages";

interface InterviewGuardStateProps {
  lang: Lang;
  code: string;
}

export function InterviewGuardState({ lang, code }: InterviewGuardStateProps) {
  const t = getInterviewMessages(lang).guard;

  return (
    <Card>
      <h1 className="text-2xl font-bold text-primary-dark mb-4">
        {t.title}
      </h1>
      <p className="text-primary-darkest mb-6">
        {t.body}
      </p>
      <Link href={`/session/${code}`}>
        <Button fullWidth>
          {t.recoverButton}
        </Button>
      </Link>
    </Card>
  );
}
