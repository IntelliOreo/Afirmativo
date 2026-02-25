"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Alert } from "@components/Alert";
import { Card } from "@components/Card";
import { disclaimerContent } from "../../../content/disclaimer";

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "";

export default function DisclaimerPage() {
  const router = useRouter();
  const [agreed, setAgreed] = useState(false);
  const [lang, setLang] = useState<"es" | "en">("es");

  // Fire Neon warm-up on page load — wakes the DB before user reaches /pay
  useEffect(() => {
    fetch(`${API_URL}/api/v1/health`).catch(() => {
      // Best-effort; silently ignore errors
    });
  }, []);

  const t = disclaimerContent[lang];

  return (
    <div className="flex flex-col min-h-screen">
      <NavHeader
        lang={lang}
        onToggleLang={() => setLang(lang === "es" ? "en" : "es")}
      />

      <main className="flex-1 bg-base-lightest">
        <div className="max-w-2xl mx-auto px-4 py-12">
          <h1 className="text-3xl font-bold text-primary-dark mb-6">
            {t.title}
          </h1>

          <Card className="mb-6">
            <div className="max-h-64 overflow-y-auto pr-2">
              <ul className="space-y-4">
                {t.bullets.map((item, i) => (
                  <li key={i} className="flex gap-3 text-primary-darkest">
                    <span className="text-primary-dark font-bold mt-0.5">•</span>
                    <span>{item}</span>
                  </li>
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
            onClick={() => agreed && router.push("/pay")}
          >
            {t.cta}
          </Button>
        </div>
      </main>

      <Footer />
    </div>
  );
}
