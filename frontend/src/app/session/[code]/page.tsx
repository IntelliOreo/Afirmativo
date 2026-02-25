"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import { Alert } from "@components/Alert";
import { Input } from "@components/Input";

type View = "loading" | "hub" | "recovery";

export default function SessionPage() {
  const params = useParams();
  const router = useRouter();
  const code = params.code as string;

  const [lang, setLang] = useState<"es" | "en">("es");
  const [view, setView] = useState<View>("loading");
  const [pin, setPin] = useState("");
  const [displayPin, setDisplayPin] = useState("••••••");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  // After hydration, check for existing session cookie
  useEffect(() => {
    const cookiePin = document.cookie
      .split("; ")
      .find((row) => row.startsWith("session_pin="))
      ?.split("=")[1];

    if (cookiePin) {
      setDisplayPin(cookiePin);
      setView("hub");
    } else {
      setView("recovery");
    }
  }, []);

  async function handleRecover() {
    if (!pin.trim() || submitting) return;
    setSubmitting(true);
    setError("");

    // TEMP: bypass API — accept any PIN for testing.
    // TODO: remove this block and uncomment the real API call before launch.
    document.cookie = `session_pin=${pin.trim()}; path=/`;
    setDisplayPin(pin.trim());
    setPin("");
    setView("hub");
    setSubmitting(false);
    return;

    // --- real recovery (disabled until backend is ready) ---
    // try {
    //   const res = await fetch(`${API_URL}/api/v1/session/recover`, {
    //     method: "POST",
    //     headers: { "Content-Type": "application/json" },
    //     body: JSON.stringify({ sessionCode: code, pin: pin.trim() }),
    //     credentials: "include",
    //   });
    //   const data = await res.json();
    //   if (res.ok) {
    //     setDisplayPin(pin.trim());
    //     setPin("");
    //     setView("hub");
    //   } else if (res.status === 404) {
    //     setError(
    //       lang === "es"
    //         ? "Código de sesión no encontrado. / Session code not found."
    //         : "Session code not found. / Código de sesión no encontrado."
    //     );
    //   } else {
    //     setError(
    //       lang === "es"
    //         ? "PIN incorrecto. Intente de nuevo. / Incorrect PIN. Please try again."
    //         : "Incorrect PIN. Please try again. / PIN incorrecto."
    //     );
    //   }
    // } catch {
    //   setError(
    //     lang === "es"
    //       ? "Error de conexión. Intente de nuevo. / Connection error. Please try again."
    //       : "Connection error. Please try again. / Error de conexión."
    //   );
    // } finally {
    //   setSubmitting(false);
    // }
  }

  return (
    <div className="flex flex-col min-h-screen">
      <NavHeader
        lang={lang}
        onToggleLang={() => setLang(lang === "es" ? "en" : "es")}
      />

      <main className="flex-1 bg-base-lightest">
        <div className="max-w-lg mx-auto px-4 py-12">

          {view === "loading" && (
            <p className="text-primary-darkest">
              {lang === "es" ? "Cargando..." : "Loading..."}
            </p>
          )}

          {view === "recovery" && (
            <>
              <h1 className="text-3xl font-bold text-primary-dark mb-2">
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
                  <span className="text-primary-dark font-bold tracking-widest">
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
              <h1 className="text-3xl font-bold text-primary-dark mb-2">
                {lang === "es" ? "Su sesión" : "Your session"}
              </h1>
              <p className="text-primary-darkest mb-8">
                {lang === "es"
                  ? "Guarde este código y PIN. Los necesitará para recuperar su sesión."
                  : "Save this code and PIN. You will need them to recover your session."}
              </p>

              <Card className="mb-6">
                <div className="space-y-4">
                  <div>
                    <p className="text-sm font-semibold text-gray-500 uppercase tracking-wide mb-1">
                      {lang === "es" ? "Código de sesión" : "Session code"}
                    </p>
                    <p className="text-2xl font-bold text-primary-dark tracking-widest">
                      {code}
                    </p>
                  </div>
                  <div className="border-t border-base-lighter pt-4">
                    <p className="text-sm font-semibold text-gray-500 uppercase tracking-wide mb-1">
                      PIN
                    </p>
                    <p className="text-2xl font-bold text-primary-dark tracking-widest">
                      {displayPin}
                    </p>
                  </div>
                </div>
              </Card>

              <Alert variant="warning" className="mb-8">
                {lang === "es"
                  ? "Tome una captura de pantalla de este código y PIN antes de continuar. Los datos se eliminan después de 48 horas."
                  : "Take a screenshot of this code and PIN before continuing. Data is deleted after 48 hours."}
              </Alert>

              <Button fullWidth onClick={() => router.push(`/interview/${code}`)}>
                {lang === "es" ? "Comenzar entrevista" : "Begin interview"}
              </Button>
            </>
          )}

        </div>
      </main>

      <Footer />
    </div>
  );
}
