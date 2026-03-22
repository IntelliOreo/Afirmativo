"use client";

import { Suspense, useEffect, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Alert } from "@components/Alert";
import { api } from "@/lib/api";
import { withLang } from "@/lib/language";
import { writePin } from "@/lib/storage/sessionPinStore";
import { useLanguage } from "@/lib/useLanguage";
import { getPayMessages } from "@/messages/payMessages";

const maxPollAttempts = 10;
const pollDelayMs = 1000;

function PaymentSuccessPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const requestedLang = searchParams.get("lang");
  const checkoutSessionID = searchParams.get("session_id") ?? "";
  const { lang, setLang, initialized } = useLanguage({ requestedLang });
  const t = getPayMessages(lang);
  const [error, setError] = useState("");
  const startedCheckoutSessionRef = useRef<string | null>(null);

  useEffect(() => {
    if (!initialized) {
      return;
    }
    if (!checkoutSessionID) {
      setError(t.missingCheckoutSession);
      return;
    }
    if (startedCheckoutSessionRef.current === checkoutSessionID) {
      return;
    }
    startedCheckoutSessionRef.current = checkoutSessionID;

    let cancelled = false;
    let timeoutID: ReturnType<typeof setTimeout> | null = null;
    let attempts = 0;

    async function poll() {
      attempts += 1;
      try {
        const result = await api<{ status?: string; session_code?: string; pin?: string; code?: string }>(
          `/api/payment/checkout-sessions/${encodeURIComponent(checkoutSessionID)}`,
        );

        if (cancelled) return;

        if (result.ok && result.data?.status === "ready" && result.data.session_code && result.data.pin) {
          writePin(result.data.session_code, result.data.pin);
          router.replace(withLang(`/session/${result.data.session_code}`, lang));
          return;
        }

        if (result.status === 202 || result.data?.status === "pending") {
          if (attempts >= maxPollAttempts) {
            setError(t.paymentStatusTimeout);
            return;
          }
          timeoutID = setTimeout(() => {
            void poll();
          }, pollDelayMs);
          return;
        }

        if (result.status === 410 || result.data?.code === "PAYMENT_REVEAL_EXPIRED") {
          setError(t.paymentRevealExpired);
          return;
        }

        setError(t.paymentStatusFailed);
      } catch {
        if (!cancelled) {
          setError(t.paymentStatusNetworkError);
        }
      }
    }

    void poll();

    return () => {
      cancelled = true;
      if (timeoutID) {
        clearTimeout(timeoutID);
      }
    };
  }, [checkoutSessionID, initialized, lang, router, t.missingCheckoutSession, t.paymentRevealExpired, t.paymentStatusFailed, t.paymentStatusNetworkError, t.paymentStatusTimeout]);

  return (
    <div className="flex flex-col min-h-screen">
      <NavHeader
        lang={lang}
        onToggleLang={() => setLang(lang === "es" ? "en" : "es")}
      />

      <main className="flex-1 bg-base-lightest">
        <div className="max-w-lg mx-auto px-4 py-8 sm:py-12">
          <h1 className="text-2xl sm:text-3xl font-bold text-primary-dark mb-2">
            {t.successHeading}
          </h1>
          <p className="text-primary-darkest mb-6">
            {t.successBody}
          </p>

          {!error && (
            <div className="rounded-lg bg-white border border-base-lighter px-4 py-6 text-center">
              <div className="mx-auto mb-4 h-10 w-10 rounded-full border-4 border-primary border-t-transparent animate-spin" />
              <p className="text-primary-darkest">
                {t.successPending}
              </p>
            </div>
          )}

          {error && (
            <>
              <Alert variant="error" className="mb-4">
                {error}
              </Alert>
              <Button fullWidth onClick={() => router.push(withLang("/pay", lang))}>
                {t.returnToPay}
              </Button>
            </>
          )}
        </div>
      </main>

      <Footer />
    </div>
  );
}

export default function PaymentSuccessPage() {
  return (
    <Suspense fallback={null}>
      <PaymentSuccessPageContent />
    </Suspense>
  );
}
