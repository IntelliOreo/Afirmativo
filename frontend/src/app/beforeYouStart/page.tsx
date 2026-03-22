"use client";

import { Suspense, useState, useEffect } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Alert } from "@components/Alert";
import { Card } from "@components/Card";
import { beforeYouStartContent } from "../../../content/beforeYouStart";
import { withLang } from "@/lib/language";
import { useLanguage } from "@/lib/useLanguage";

function BeforeYouStartPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const requestedLang = searchParams.get("lang");
  const [agreed, setAgreed] = useState(false);
  const { lang, setLang } = useLanguage({ requestedLang });

  // Fire Neon warm-up on page load — wakes the DB before user reaches /pay
  useEffect(() => {
    fetch("/api/health").catch(() => {
      // Best-effort; silently ignore errors
    });
  }, []);
  const t = beforeYouStartContent[lang];

  return (
    <div className="flex flex-col min-h-screen">
      <NavHeader
        lang={lang}
        onToggleLang={() => setLang(lang === "es" ? "en" : "es")}
      />

      <main className="flex-1 bg-base-lightest">
        <div className="max-w-2xl mx-auto px-4 py-8 sm:py-12">
          <h1 className="text-2xl sm:text-3xl font-bold text-primary-dark mb-6">
            {t.title}
          </h1>

          <Card className="mb-6">
            <div className="max-h-64 overflow-y-auto pr-2">
              <ul className="list-disc list-inside space-y-3 text-primary-darkest leading-relaxed">
                {t.bullets.map((item, i) => (
                  <li key={i} className="text-base">{item}</li>
                ))}
              </ul>
            </div>
          </Card>

          <Alert variant="warning" className="mb-6">
            {t.warningAlert}
          </Alert>

          <label className="flex items-start gap-3 cursor-pointer mb-8">
            <input
              type="checkbox"
              checked={agreed}
              onChange={(e) => setAgreed(e.target.checked)}
              className="mt-1 h-5 w-5 rounded border-base-lighter accent-primary cursor-pointer"
            />
            <span className="text-primary-darkest">{t.checkLabel}</span>
          </label>

          <Button
            fullWidth
            disabled={!agreed}
            onClick={() => agreed && router.push(withLang("/pay", lang))}
          >
            {t.cta}
          </Button>
        </div>
      </main>

      <Footer />
    </div>
  );
}

export default function BeforeYouStartPage() {
  return (
    <Suspense fallback={null}>
      <BeforeYouStartPageContent />
    </Suspense>
  );
}
