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
import { getCommonMessages } from "@/messages/commonMessages";
import { getLandingMessages, getLandingVerifyErrorMessage } from "@/messages/landingMessages";

export default function LandingPage() {
  const router = useRouter();
  const { lang, setLang } = useLanguage({ requestedLang: null });
  const [sessionCodeInput, setSessionCodeInput] = useState("");
  const [pinInput, setPinInput] = useState("");
  const [verificationError, setVerificationError] = useState("");
  const [isVerifying, setIsVerifying] = useState(false);
  const t = getLandingMessages(lang);
  const common = getCommonMessages(lang);

  async function handleVerifySession() {
    if (!sessionCodeInput.trim() || !pinInput.trim()) return;
    setIsVerifying(true);
    setVerificationError("");

    const normalizedSessionCode = sessionCodeInput.trim();
    const normalizedPin = pinInput.trim();
    const result = await verifySession(normalizedSessionCode, normalizedPin);
    if (result.ok) {
      router.push(withLang(`/interview/${normalizedSessionCode}`, lang));
    } else {
      setVerificationError(getLandingVerifyErrorMessage(lang, result.reason));
    }
    setIsVerifying(false);
  }
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
                {t.adminLink}
              </Link>
            </p>
          )}

          {/* Resume session */}
          <div className="border-t border-base-lighter pt-8">
            <h2 className="text-lg font-bold text-primary-dark mb-2">
              {t.resumeHeading}
            </h2>
            <p className="text-sm text-gray-600 mb-4">
              {t.resumeBody}
            </p>
            <form
              onSubmit={(e) => { e.preventDefault(); handleVerifySession(); }}
              className="space-y-3"
            >
              <Input
                label={common.sessionCodeLabel}
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
                {isVerifying ? common.verifying : t.resumeButton}
              </Button>
            </form>
          </div>
        </div>
      </main>

      <Footer />
    </div>
  );
}
