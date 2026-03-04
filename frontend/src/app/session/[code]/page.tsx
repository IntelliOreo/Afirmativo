"use client";

import { Suspense, useState, useEffect, useCallback } from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import { Input } from "@components/Input";
import { api } from "@/lib/api";
import { parseLang, resolveLang, writeStoredLang } from "@/lib/language";

type View = "loading" | "hub" | "recovery";

function SessionPageContent() {
  const params = useParams();
  const router = useRouter();
  const searchParams = useSearchParams();
  const code = params.code as string;
  const requestedLang = searchParams.get("lang");

  const [lang, setLang] = useState<"es" | "en">(() => resolveLang(requestedLang));
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
      if (typeof window !== "undefined") {
        writeStoredLang(selectedLang);
        sessionStorage.setItem(`interview_lang_${code}`, selectedLang);
      }
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
    writeStoredLang(lang);
  }, [lang]);

  useEffect(() => {
    const langFromQuery = parseLang(requestedLang);
    if (langFromQuery) {
      setLang(langFromQuery);
    }
  }, [requestedLang]);

  const verifySession = useCallback(
    async (sessionCode: string, sessionPin: string) => {
      const result = await api<{ session: { interview_started_at?: string } }>("/api/session/verify", {
        method: "POST",
        body: { sessionCode, pin: sessionPin },
      });
      if (!result.ok) return { ok: false as const, status: result.status };
      return { ok: true as const, session: result.data!.session };
    },
    []
  );

  useEffect(() => {
    async function init() {
      // 1. Check sessionStorage — set by /pay after coupon validate
      const storedPin = sessionStorage.getItem(`pin_${code}`);
      if (storedPin) {
        sessionStorage.removeItem(`pin_${code}`);
        try {
          const result = await verifySession(code, storedPin);
          if (result.ok) {
            setDisplayPin(storedPin);
            if (result.session.interview_started_at) {
              goToInterview(lang, true);
              return;
            }
            setView("hub");
            return;
          }

          setError(
            result.status === 410
              ? lang === "es"
                ? "Esta sesión ha expirado. / This session has expired."
                : "This session has expired. / Esta sesión ha expirado."
              : ""
          );
          setView("recovery");
          return;
        } catch {
          setError(
            lang === "es"
              ? "Error de conexión. Intente de nuevo. / Connection error. Please try again."
              : "Connection error. Please try again. / Error de conexión."
          );
          setView("recovery");
          return;
        }
      }

      // 2. No local PIN available — show recovery form.
      setView("recovery");
    }
    init();
  }, [code, verifySession, goToInterview, lang]);

  function getRecoveryUrl() {
    if (typeof window === "undefined") return "";
    return `${window.location.origin}/session/${code}`;
  }

  async function handleCopyAll() {
    const url = getRecoveryUrl();
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

  async function handleRecover() {
    if (!pin.trim() || submitting) return;
    setSubmitting(true);
    setError("");

    try {
      const result = await verifySession(code, pin.trim());
      if (result.ok) {
        if (result.session.interview_started_at) {
          goToInterview(lang, true);
          return;
        }
        setDisplayPin(pin.trim());
        setPin("");
        setView("hub");
      } else if (result.status === 404) {
        setError(
          lang === "es"
            ? "Código de sesión no encontrado. / Session code not found."
            : "Session code not found. / Código de sesión no encontrado."
        );
      } else if (result.status === 410) {
        setError(
          lang === "es"
            ? "Esta sesión ha expirado. / This session has expired."
            : "This session has expired. / Esta sesión ha expirado."
        );
      } else {
        setError(
          lang === "es"
            ? "PIN incorrecto. Intente de nuevo. / Incorrect PIN. Please try again."
            : "Incorrect PIN. Please try again. / PIN incorrecto."
        );
      }
    } catch {
      setError(
        lang === "es"
          ? "Error de conexión. Intente de nuevo. / Connection error. Please try again."
          : "Connection error. Please try again. / Error de conexión."
      );
    } finally {
      setSubmitting(false);
    }
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

          {view === "recovery" && (
            <>
              <h1 className="text-2xl sm:text-3xl font-bold text-primary-dark mb-2">
                {lang === "es" ? "Recuperar sesión" : "Recover session"}
              </h1>
              <p className="text-primary-darkest mb-8">
                {lang === "es"
                  ? "Ingrese su PIN de 6 dígitos para recuperar el acceso a su sesión."
                  : "Enter your 6-digit PIN to recover access to your session."}
              </p>

              <Card className="mb-4">
                <p className="text-sm font-semibold text-gray-500 uppercase tracking-wide mb-4">
                  {lang === "es" ? "Código de sesión" : "Session code"}:{" "}
                  <span className="text-primary-dark font-bold tracking-wide break-all">
                    {code}
                  </span>
                </p>
                <form
                  onSubmit={(e) => { e.preventDefault(); handleRecover(); }}
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
                      : lang === "es" ? "Recuperar sesión" : "Recover session"}
                  </Button>
                </form>
              </Card>
            </>
          )}

          {view === "hub" && (
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
                    ? "Si pierde la conexión o desea volver más tarde, use esta información para recuperar su sesión. Guárdela o tome una captura de pantalla."
                    : "If you lose your connection or want to come back later, use this info to recover your session. Save it or take a screenshot."}
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
                      {getRecoveryUrl()}
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
