"use client";

import { Suspense, useState, useEffect, useCallback, useRef } from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import { Input } from "@components/Input";
import { writeInterviewLang } from "@/lib/storage/languageStore";
import { readAndConsumePin } from "@/lib/storage/sessionPinStore";
import { checkSessionAccess, verifySession } from "@/lib/sessionService";
import { useLanguage } from "@/lib/useLanguage";
import { getCommonMessages } from "@/messages/commonMessages";
import { getSessionMessages, getSessionVerifyErrorMessage } from "@/messages/sessionMessages";

type View = "loading" | "ready" | "verification";

function SessionPageContent() {
  const params = useParams();
  const router = useRouter();
  const searchParams = useSearchParams();
  const code = params.code as string;
  const requestedLang = searchParams.get("lang");

  const { lang, setLang, initialized } = useLanguage({ requestedLang, sessionCode: code });
  const t = getSessionMessages(lang);
  const common = getCommonMessages(lang);
  const [view, setView] = useState<View>("loading");
  const [pin, setPin] = useState("");
  const [displayPin, setDisplayPin] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [copied, setCopied] = useState(false);
  const startedBootstrapSessionRef = useRef<string | null>(null);

  const interviewUrlForLang = useCallback(
    (selectedLang: "es" | "en") => `/interview/${code}?lang=${selectedLang}`,
    [code],
  );

  const goToInterview = useCallback(
    (selectedLang: "es" | "en", replace = false) => {
      writeInterviewLang(code, selectedLang);
      const nextUrl = interviewUrlForLang(selectedLang);
      if (replace) {
        router.replace(nextUrl);
        return;
      }
      router.push(nextUrl);
    },
    [code, interviewUrlForLang, router],
  );

  useEffect(() => {
    if (!initialized) {
      return;
    }
    if (startedBootstrapSessionRef.current === code) {
      return;
    }
    startedBootstrapSessionRef.current = code;

    let cancelled = false;

    async function init() {
      const storedPin = readAndConsumePin(code);
      if (storedPin) {
        const result = await verifySession(code, storedPin);
        if (cancelled) {
          return;
        }
        if (result.ok) {
          setDisplayPin(storedPin);
          if (result.interviewStartedAt) {
            goToInterview(lang, true);
            return;
          }
          setView("ready");
          return;
        }

        if (result.reason === "expired") {
          setError(getSessionVerifyErrorMessage(lang, result.reason));
        } else if (result.reason === "network") {
          setError(t.networkError);
        } else {
          setError(getSessionVerifyErrorMessage(lang, result.reason));
        }
        setView("verification");
        return;
      }

      const accessResult = await checkSessionAccess(code);
      if (cancelled) {
        return;
      }
      if (accessResult.ok) {
        goToInterview(lang, true);
        return;
      }

      if (accessResult.reason === "network") {
        setError(t.networkError);
      } else if (accessResult.reason === "unknown") {
        setError(t.genericAccessError);
      }

      setView("verification");
    }
    init();

    return () => {
      cancelled = true;
    };
  }, [code, goToInterview, initialized, lang, t.genericAccessError, t.networkError]);

  function getResumeUrl() {
    if (typeof window === "undefined") return "";
    return `${window.location.origin}/session/${code}`;
  }

  async function handleCopyAll() {
    const url = getResumeUrl();
    const text = [
      `${common.sessionCodeLabel}: ${code}`,
      `${common.pinLabel}: ${displayPin}`,
      `${common.linkLabel}: ${url}`,
    ].join("\n");

    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const textarea = document.createElement("textarea");
      textarea.value = text;
      textarea.style.position = "fixed";
      textarea.style.opacity = "0";
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand("copy");
      document.body.removeChild(textarea);
    }
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  async function handleVerifySession() {
    if (!pin.trim() || submitting) return;
    setSubmitting(true);
    setError("");

    const result = await verifySession(code, pin.trim());
    if (result.ok) {
      if (result.interviewStartedAt) {
        goToInterview(lang, true);
        return;
      }
      setDisplayPin(pin.trim());
      setPin("");
      setView("ready");
    } else {
      setError(getSessionVerifyErrorMessage(lang, result.reason));
    }
    setSubmitting(false);
  }

  return (
    <div className="flex flex-col min-h-screen">
      <NavHeader
        lang={lang}
        onToggleLang={() => setLang(lang === "es" ? "en" : "es")}
      />

      <main className="flex-1 bg-base-lightest">
        <div className="max-w-lg mx-auto px-4 py-8 sm:py-12">

          {view === "loading" && (
            <p className="text-primary-darkest">
              {t.loading}
            </p>
          )}

          {view === "verification" && (
            <>
              <h1 className="text-2xl sm:text-3xl font-bold text-primary-dark mb-2">
                {t.resumeHeading}
              </h1>
              <p className="text-primary-darkest mb-8">
                {t.resumeBody}
              </p>

              <Card className="mb-4">
                <p className="text-sm font-semibold text-gray-500 uppercase tracking-wide mb-4">
                  {common.sessionCodeLabel}:{" "}
                  <span className="text-primary-dark font-bold tracking-wide break-all">
                    {code}
                  </span>
                </p>
                <form
                  onSubmit={(e) => { e.preventDefault(); handleVerifySession(); }}
                  className="space-y-4"
                >
                  <Input
                    label="PIN"
                    placeholder="123456"
                    value={pin}
                    onChange={(e) => setPin(e.target.value)}
                    inputMode="numeric"
                    maxLength={6}
                    autoComplete="one-time-code"
                    error={error}
                  />
                  <Button
                    type="submit"
                    fullWidth
                    disabled={submitting || !pin.trim()}
                  >
                    {submitting ? common.verifying : t.resumeButton}
                  </Button>
                </form>
              </Card>
            </>
          )}

          {view === "ready" && (
            <>
              <h1 className="text-2xl sm:text-3xl font-bold text-primary-dark mb-2">
                {t.readyHeading}
              </h1>
              <p className="text-primary-darkest mb-6">
                {t.readyBody}
              </p>

              <Button fullWidth className="mb-8" onClick={() => goToInterview(lang)}>
                {t.beginInterview}
              </Button>

              <Card className="mb-4">
                <p className="text-sm text-gray-600 mb-4">
                  {t.recoveryInfo}
                </p>

                <div className="space-y-3 mb-4">
                  <div className="flex flex-col items-start gap-1 bg-base-lightest rounded px-3 py-2 sm:flex-row sm:items-center sm:justify-between">
                    <span className="text-xs font-semibold text-gray-500 uppercase tracking-wide">
                      {common.sessionCodeLabel}
                    </span>
                    <span className="font-bold text-primary-dark tracking-wide break-all w-full text-left sm:w-auto sm:text-right">
                      {code}
                    </span>
                  </div>
                  <div className="flex flex-col items-start gap-1 bg-base-lightest rounded px-3 py-2 sm:flex-row sm:items-center sm:justify-between">
                    <span className="text-xs font-semibold text-gray-500 uppercase tracking-wide">
                      {common.pinLabel}
                    </span>
                    <span className="font-bold text-primary-dark tracking-wide break-all w-full text-left sm:w-auto sm:text-right">
                      {displayPin}
                    </span>
                  </div>
                  <div className="flex flex-col items-start gap-1 bg-base-lightest rounded px-3 py-2 sm:flex-row sm:items-center sm:justify-between">
                    <span className="text-xs font-semibold text-gray-500 uppercase tracking-wide">
                      {common.linkLabel}
                    </span>
                    <span className="font-bold text-primary text-sm break-all w-full text-left sm:w-auto sm:text-right">
                      {getResumeUrl()}
                    </span>
                  </div>
                </div>

                <Button
                  fullWidth
                  variant="secondary"
                  onClick={handleCopyAll}
                >
                  {copied ? common.copied : common.copyAll}
                </Button>
              </Card>
            </>
          )}

        </div>
      </main>

      <Footer />
    </div>
  );
}

export default function SessionPage() {
  return (
    <Suspense fallback={null}>
      <SessionPageContent />
    </Suspense>
  );
}
