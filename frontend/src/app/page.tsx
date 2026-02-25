"use client";

import { useState } from "react";
import Link from "next/link";
import { NavHeader } from "../../components/NavHeader";
import { Footer } from "../../components/Footer";
import { Button } from "../../components/Button";
import { Card } from "../../components/Card";

export default function LandingPage() {
  const [lang, setLang] = useState<"es" | "en">("es");

  const content = {
    es: {
      headline: "Prepárese para su entrevista afirmativa de asilo",
      subheadline:
        "Una herramienta de práctica confidencial con reporte bilingüe.",
      steps: [
        "Lea el descargo de responsabilidad y acepte los términos",
        "Ingrese un cupón o pague en línea",
        "Realice la entrevista simulada — una pregunta a la vez",
        "Descargue su reporte de evaluación bilingüe",
      ],
      cta: "Comenzar",
      note: "Sin cuenta. Sin inicio de sesión. Su sesión es completamente anónima.",
    },
    en: {
      headline: "Prepare for your affirmative asylum interview",
      subheadline:
        "A confidential practice tool with a bilingual assessment report.",
      steps: [
        "Read the disclaimer and agree to the terms",
        "Enter a coupon code or pay online",
        "Complete the simulated interview — one question at a time",
        "Download your bilingual assessment report",
      ],
      cta: "Get Started",
      note: "No account. No login. Your session is completely anonymous.",
    },
  };

  const t = content[lang];

  return (
    <div className="flex flex-col min-h-screen">
      <NavHeader
        lang={lang}
        onToggleLang={() => setLang(lang === "es" ? "en" : "es")}
      />

      <main className="flex-1 bg-base-lightest">
        <div className="max-w-3xl mx-auto px-4 py-12">
          <h1 className="text-3xl font-bold text-primary-dark mb-4">
            {t.headline}
          </h1>
          <p className="text-lg text-primary-darkest mb-8">{t.subheadline}</p>

          <Card className="mb-8">
            <ol className="list-decimal list-inside space-y-3 text-primary-darkest">
              {t.steps.map((step, i) => (
                <li key={i}>{step}</li>
              ))}
            </ol>
          </Card>

          <div className="mb-6">
            <Link href="/disclaimer">
              <Button fullWidth>{t.cta}</Button>
            </Link>
          </div>

          <p className="text-sm text-gray-600 text-center">{t.note}</p>
        </div>
      </main>

      <Footer />
    </div>
  );
}
