"use client";

import { Suspense, useState, useEffect, useCallback } from "react";
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

type View = "loading" | "ready" | "verification";

function networkError(lang: "es" | "en"): string {
  return lang === "es"
    ? "Error de conexión. Intente de nuevo. / Connection error. Please try again."
    : "Connection error. Please try again. / Error de conexión.";
}

function genericAccessError(lang: "es" | "en"): string {
  return lang === "es"
    ? "No se pudo verificar la sesión. Intente de nuevo. / Could not verify session. Please try again."
    : "Could not verify session. Please try again. / No se pudo verificar la sesión. Intente de nuevo.";
}

function SessionPageContent() {
  const params = useParams();
  const router = useRouter();
  const searchParams = useSearchParams();
  const code = params.code as string;
  const requestedLang = searchParams.get("lang");

  const { lang, setLang } = useLanguage({ requestedLang, sessionCode: code });
  const [view, setView] = useState<View>("loading");
  const [pin, setPin] = useState("");
  const [displayPin, setDisplayPin] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [copied, setCopied] = useState(false);

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
    async function init() {
      const storedPin = readAndConsumePin(code);
      if (storedPin) {
        const result = await verifySession(code, storedPin);
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
          setError(
            lang === "es"
              ? "Esta sesión ha expirado. / This session has expired."
              : "This session has expired. / Esta sesión ha expirado."
          );
        } else if (result.reason === "network") {
          setError(networkError(lang));
        }
        setView("verification");
        return;
      }

      const accessResult = await checkSessionAccess(code);
      if (accessResult.ok) {
        goToInterview(lang, true);
        return;
      }

      if (accessResult.reason === "network") {
        setError(networkError(lang));
      } else if (accessResult.reason === "unknown") {
        setError(genericAccessError(lang));
      }

      setView("verification");
    }
    init();
  }, [code, goToInterview, lang]);

  function getResumeUrl() {
    if (typeof window === "undefined") return "";
    return `${window.location.origin}/session/${code}`;
  }

  async function handleCopyAll() {
    const url = getResumeUrl();
    const text = [
      `${lang === "es" ? "Código de sesión" : "Session code"}: ${code}`,
      `PIN: ${displayPin}`,
      `${lang === "es" ? "Enlace" : "Link"}: ${url}`,
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
    } else if (result.reason === "not_found") {
      setError(
        lang === "es"
          ? "Código de sesión no encontrado. / Session code not found."
          : "Session code not found. / Código de sesión no encontrado."
      );
    } else if (result.reason === "expired") {
      setError(
        lang === "es"
          ? "Esta sesión ha expirado. / This session has expired."
          : "This session has expired. / Esta sesión ha expirado."
      );
    } else if (result.reason === "network") {
      setError(networkError(lang));
    } else {
      setError(
        lang === "es"
          ? "PIN incorrecto. Intente de nuevo. / Incorrect PIN. Please try again."
          : "Incorrect PIN. Please try again. / PIN incorrecto."
      );
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
              {lang === "es" ? "Cargando..." : "Loading..."}
            </p>
          )}

          {view === "verification" && (
            <>
              <h1 className="text-2xl sm:text-3xl font-bold text-primary-dark mb-2">
                {lang === "es" ? "Reanudar sesión" : "Resume session"}
              </h1>
              <p className="text-primary-darkest mb-8">
                {lang === "es"
                  ? "Ingrese su PIN de 6 dígitos para reanudar el acceso a su sesión."
                  : "Enter your 6-digit PIN to resume access to your session."}
              </p>

              <Card className="mb-4">
                <p className="text-sm font-semibold text-gray-500 uppercase tracking-wide mb-4">
                  {lang === "es" ? "Código de sesión" : "Session code"}:{" "}
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
                    {submitting
                      ? lang === "es" ? "Verificando..." : "Verifying..."
                      : lang === "es" ? "Reanudar sesión" : "Resume session"}
                  </Button>
                </form>
              </Card>
            </>
          )}

          {view === "ready" && (
            <>
              <h1 className="text-2xl sm:text-3xl font-bold text-primary-dark mb-2">
                {lang === "es" ? "Su sesión está lista" : "Your session is ready"}
              </h1>
              <p className="text-primary-darkest mb-6">
                {lang === "es"
                  ? "Puede comenzar la entrevista de inmediato con el botón de abajo."
                  : "You can start the interview right away using the button below."}
              </p>

              <Button fullWidth className="mb-8" onClick={() => goToInterview(lang)}>
                {lang === "es" ? "Comenzar entrevista" : "Begin interview"}
              </Button>

              <Card className="mb-4">
                <p className="text-sm text-gray-600 mb-4">
                  {lang === "es"
                    ? "Si pierde la conexión o desea volver más tarde, use esta información para reanudar su sesión. Guárdela o tome una captura de pantalla."
                    : "If you lose your connection or want to come back later, use this info to resume your session. Save it or take a screenshot."}
                </p>

                <div className="space-y-3 mb-4">
                  <div className="flex flex-col items-start gap-1 bg-base-lightest rounded px-3 py-2 sm:flex-row sm:items-center sm:justify-between">
                    <span className="text-xs font-semibold text-gray-500 uppercase tracking-wide">
                      {lang === "es" ? "Código" : "Code"}
                    </span>
                    <span className="font-bold text-primary-dark tracking-wide break-all w-full text-left sm:w-auto sm:text-right">
                      {code}
                    </span>
                  </div>
                  <div className="flex flex-col items-start gap-1 bg-base-lightest rounded px-3 py-2 sm:flex-row sm:items-center sm:justify-between">
                    <span className="text-xs font-semibold text-gray-500 uppercase tracking-wide">
                      PIN
                    </span>
                    <span className="font-bold text-primary-dark tracking-wide break-all w-full text-left sm:w-auto sm:text-right">
                      {displayPin}
                    </span>
                  </div>
                  <div className="flex flex-col items-start gap-1 bg-base-lightest rounded px-3 py-2 sm:flex-row sm:items-center sm:justify-between">
                    <span className="text-xs font-semibold text-gray-500 uppercase tracking-wide">
                      {lang === "es" ? "Enlace" : "Link"}
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
                  {copied
                    ? lang === "es" ? "Copiado" : "Copied"
                    : lang === "es" ? "Copiar todo" : "Copy all"}
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
