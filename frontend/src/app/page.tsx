"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { NavHeader } from "../../components/NavHeader";
import { Footer } from "../../components/Footer";
import { Button } from "../../components/Button";
import { Card } from "../../components/Card";
import { Input } from "../../components/Input";
import { isAdminToolsEnabled } from "@/lib/env";
import { withLang } from "@/lib/language";
import { verifySession } from "@/lib/sessionService";
import { useLanguage } from "@/lib/useLanguage";

export default function LandingPage() {
  const router = useRouter();
  const { lang, setLang } = useLanguage({ requestedLang: null });
  const [sessionCodeInput, setSessionCodeInput] = useState("");
  const [pinInput, setPinInput] = useState("");
  const [verificationError, setVerificationError] = useState("");
  const [isVerifying, setIsVerifying] = useState(false);

  async function handleVerifySession() {
    if (!sessionCodeInput.trim() || !pinInput.trim()) return;
    setIsVerifying(true);
    setVerificationError("");

    const normalizedSessionCode = sessionCodeInput.trim();
    const normalizedPin = pinInput.trim();
    const result = await verifySession(normalizedSessionCode, normalizedPin);
    if (result.ok) {
      router.push(withLang(`/interview/${normalizedSessionCode}`, lang));
    } else if (result.reason === "not_found") {
      setVerificationError(
        lang === "es"
          ? "Código de sesión no encontrado. / Session code not found."
          : "Session code not found. / Código de sesión no encontrado."
      );
    } else if (result.reason === "expired") {
      setVerificationError(
        lang === "es"
          ? "Esta sesión ha expirado. / This session has expired."
          : "This session has expired. / Esta sesión ha expirado."
      );
    } else if (result.reason === "network") {
      setVerificationError(
        lang === "es"
          ? "Error de conexión. Intente de nuevo. / Connection error. Please try again."
          : "Connection error. Please try again. / Error de conexión."
      );
    } else {
      setVerificationError(
        lang === "es"
          ? "PIN incorrecto. Intente de nuevo. / Incorrect PIN. Please try again."
          : "Incorrect PIN. Please try again. / PIN incorrecto."
      );
    }
    setIsVerifying(false);
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
  const showAdminLink = isAdminToolsEnabled();

  return (
    <div className="flex flex-col min-h-screen">
      <NavHeader
        lang={lang}
        onToggleLang={() => setLang(lang === "es" ? "en" : "es")}
      />

      <main className="flex-1 bg-base-lightest">
        <div className="max-w-3xl mx-auto px-4 py-8 sm:py-12">
          <h1 className="text-2xl sm:text-3xl font-bold text-primary-dark mb-4">
            {t.headline}
          </h1>
          <p className="text-base sm:text-lg text-primary-darkest mb-8">{t.subheadline}</p>

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

          {showAdminLink && (
            <p className="text-sm text-center mb-10">
              <Link href="/admin">
                Admin (dev): Limpieza DB / DB cleanup
              </Link>
            </p>
          )}

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
              onSubmit={(e) => { e.preventDefault(); handleVerifySession(); }}
              className="space-y-3"
            >
              <Input
                label={lang === "es" ? "Código de sesión" : "Session code"}
                placeholder="AP-XXXX-XXXX"
                value={sessionCodeInput}
                onChange={(e) => setSessionCodeInput(e.target.value.toUpperCase())}
                autoCapitalize="characters"
                autoCorrect="off"
              />
              <Input
                label="PIN"
                placeholder="123456"
                value={pinInput}
                onChange={(e) => setPinInput(e.target.value)}
                inputMode="numeric"
                maxLength={6}
                autoComplete="one-time-code"
                error={verificationError}
              />
              <Button
                type="submit"
                fullWidth
                variant="secondary"
                disabled={isVerifying || !sessionCodeInput.trim() || !pinInput.trim()}
              >
                {isVerifying
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
