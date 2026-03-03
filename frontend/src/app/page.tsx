"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { NavHeader } from "../../components/NavHeader";
import { Footer } from "../../components/Footer";
import { Button } from "../../components/Button";
import { Card } from "../../components/Card";
import { Input } from "../../components/Input";
import { api } from "@/lib/api";
import { resolveLang, withLang, writeStoredLang } from "@/lib/language";

export default function LandingPage() {
  const router = useRouter();
  const [lang, setLang] = useState<"es" | "en">(() => resolveLang(null));
  const [resumeCode, setResumeCode] = useState("");
  const [resumePin, setResumePin] = useState("");
  const [resumeError, setResumeError] = useState("");
  const [resumeLoading, setResumeLoading] = useState(false);

  useEffect(() => {
    writeStoredLang(lang);
  }, [lang]);

  async function handleResume() {
    if (!resumeCode.trim() || !resumePin.trim()) return;
    setResumeLoading(true);
    setResumeError("");

    try {
      const { ok, status } = await api("/api/session/verify", {
        method: "POST",
        body: { sessionCode: resumeCode.trim(), pin: resumePin.trim() },
      });

      if (ok) {
        document.cookie = `session_${resumeCode.trim()}=${resumePin.trim()}; path=/; max-age=86400; SameSite=Lax`;
        router.push(withLang(`/session/${resumeCode.trim()}`, lang));
      } else if (status === 404) {
        setResumeError(
          lang === "es"
            ? "Código de sesión no encontrado. / Session code not found."
            : "Session code not found. / Código de sesión no encontrado."
        );
      } else if (status === 410) {
        setResumeError(
          lang === "es"
            ? "Esta sesión ha expirado. / This session has expired."
            : "This session has expired. / Esta sesión ha expirado."
        );
      } else {
        setResumeError(
          lang === "es"
            ? "PIN incorrecto. Intente de nuevo. / Incorrect PIN. Please try again."
            : "Incorrect PIN. Please try again. / PIN incorrecto."
        );
      }
    } catch {
      setResumeError(
        lang === "es"
          ? "Error de conexión. Intente de nuevo. / Connection error. Please try again."
          : "Connection error. Please try again. / Error de conexión."
      );
    } finally {
      setResumeLoading(false);
    }
  }

  const content = {
    es: {
      headline: "Prepárese para su entrevista afirmativa de asilo",
      subheadline:
        "Una herramienta de práctica confidencial con reporte bilingüe.",
      steps: [
        "Lea Antes de Comenzar y acepte los términos",
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
        "Read Before You Start and agree to the terms",
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
            <Link href={withLang("/beforeYouStart", lang)}>
              <Button fullWidth>{t.cta}</Button>
            </Link>
          </div>

          <p className="text-sm text-gray-600 text-center mb-10">{t.note}</p>

          {/* Resume session */}
          <div className="border-t border-base-lighter pt-8">
            <h2 className="text-lg font-bold text-primary-dark mb-2">
              {lang === "es" ? "¿Ya tiene una sesión?" : "Already have a session?"}
            </h2>
            <p className="text-sm text-gray-600 mb-4">
              {lang === "es"
                ? "Ingrese su código de sesión y PIN para continuar."
                : "Enter your session code and PIN to continue."}
            </p>
            <form
              onSubmit={(e) => { e.preventDefault(); handleResume(); }}
              className="space-y-3"
            >
              <Input
                label={lang === "es" ? "Código de sesión" : "Session code"}
                placeholder="AP-XXXX-XXXX"
                value={resumeCode}
                onChange={(e) => setResumeCode(e.target.value.toUpperCase())}
                autoCapitalize="characters"
                autoCorrect="off"
              />
              <Input
                label="PIN"
                placeholder="123456"
                value={resumePin}
                onChange={(e) => setResumePin(e.target.value)}
                inputMode="numeric"
                maxLength={6}
                autoComplete="one-time-code"
                error={resumeError}
              />
              <Button
                type="submit"
                fullWidth
                variant="secondary"
                disabled={resumeLoading || !resumeCode.trim() || !resumePin.trim()}
              >
                {resumeLoading
                  ? lang === "es" ? "Verificando..." : "Verifying..."
                  : lang === "es" ? "Reanudar sesión" : "Resume session"}
              </Button>
            </form>
          </div>
        </div>
      </main>

      <Footer />
    </div>
  );
}
