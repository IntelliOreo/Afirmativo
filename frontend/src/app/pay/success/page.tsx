"use client";

import { Suspense, useEffect, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import { Alert } from "@components/Alert";
import { api } from "@/lib/api";
import { withLang } from "@/lib/language";
import { writePin } from "@/lib/storage/sessionPinStore";
import { useLanguage } from "@/lib/useLanguage";
import { getCommonMessages } from "@/messages/commonMessages";
import { getPayMessages } from "@/messages/payMessages";
import { getSessionCouponUsageSummary } from "@/messages/sessionMessages";

const maxPollAttempts = 10;
const pollDelayMs = 1000;

type CouponReady = {
  code: string;
  maxUses: number;
  currentUses: number;
};

function PaymentSuccessPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const requestedLang = searchParams.get("lang");
  const checkoutSessionID = searchParams.get("session_id") ?? "";
  const { lang, setLang, initialized } = useLanguage({ requestedLang });
  const t = getPayMessages(lang);
  const common = getCommonMessages(lang);
  const [error, setError] = useState("");
  const [couponReady, setCouponReady] = useState<CouponReady | null>(null);
  const [copied, setCopied] = useState(false);
  const startedCheckoutSessionRef = useRef<string | null>(null);

  function getRedeemPath(couponCode: string) {
    return withLang(`/pay?coupon=${encodeURIComponent(couponCode)}`, lang);
  }

  function getRedeemUrl(couponCode: string) {
    if (typeof window === "undefined") return "";
    return `${window.location.origin}${getRedeemPath(couponCode)}`;
  }

  function getCouponRevealText(coupon: CouponReady) {
    return [
      `${t.couponLabel}: ${coupon.code}`,
      getSessionCouponUsageSummary(lang, coupon.currentUses, coupon.maxUses),
      `${common.linkLabel}: ${getRedeemUrl(coupon.code)}`,
    ].join("\n");
  }

  async function handleCopyCouponInfo() {
    if (!couponReady) return;
    const text = getCouponRevealText(couponReady);

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

  function handleEmailCouponInfo() {
    if (!couponReady) return;
    const params = new URLSearchParams({
      subject: t.couponEmailSubject,
      body: getCouponRevealText(couponReady),
    });
    window.location.href = `mailto:?${params.toString()}`;
  }

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
        const result = await api<{
          status?: string;
          product_type?: string;
          session_code?: string;
          pin?: string;
          coupon_code?: string;
          coupon_max_uses?: number;
          coupon_current_uses?: number;
          code?: string;
        }>(
          `/api/payment/checkout-sessions/${encodeURIComponent(checkoutSessionID)}`,
        );

        if (cancelled) return;

        if (result.ok && result.data?.status === "ready") {
          if (result.data.product_type === "direct_session" && result.data.session_code && result.data.pin) {
            writePin(result.data.session_code, result.data.pin);
            router.replace(withLang(`/session/${result.data.session_code}`, lang));
            return;
          }

          if (
            result.data.product_type === "coupon_pack_10" &&
            typeof result.data.coupon_code === "string" &&
            typeof result.data.coupon_max_uses === "number"
          ) {
            setCouponReady({
              code: result.data.coupon_code,
              maxUses: result.data.coupon_max_uses,
              currentUses: typeof result.data.coupon_current_uses === "number" ? result.data.coupon_current_uses : 0,
            });
            return;
          }
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
            {couponReady ? t.couponReadyHeading : t.successHeading}
          </h1>
          <p className="text-primary-darkest mb-6">
            {couponReady ? t.couponReadyBody : t.successBody}
          </p>

          {!error && !couponReady && (
            <div className="rounded-lg bg-white border border-base-lighter px-4 py-6 text-center">
              <div className="mx-auto mb-4 h-10 w-10 rounded-full border-4 border-primary border-t-transparent animate-spin" />
              <p className="text-primary-darkest">
                {t.successPending}
              </p>
            </div>
          )}

          {couponReady && (
            <>
              <Button fullWidth className="mb-8" onClick={() => router.push(getRedeemPath(couponReady.code))}>
                {t.redeemNow}
              </Button>
              <Card>
                <p className="text-sm text-gray-600 mb-4">
                  {t.couponRecoveryInfo}
                </p>
                <div className="space-y-3 mb-4">
                  <div className="flex flex-col items-start gap-1 bg-base-lightest rounded px-3 py-2 sm:flex-row sm:items-center sm:justify-between">
                    <span className="text-xs font-semibold text-gray-500 uppercase tracking-wide">
                      {t.couponLabel}
                    </span>
                    <span className="font-bold text-primary-dark tracking-wide break-all w-full text-left sm:w-auto sm:text-right">
                      {couponReady.code}
                    </span>
                  </div>
                  <div className="bg-base-lightest rounded px-3 py-2">
                    <p className="text-sm text-primary-darkest">
                      {getSessionCouponUsageSummary(lang, couponReady.currentUses, couponReady.maxUses)}
                    </p>
                  </div>
                  <div className="flex flex-col items-start gap-1 bg-base-lightest rounded px-3 py-2 sm:flex-row sm:items-center sm:justify-between">
                    <span className="text-xs font-semibold text-gray-500 uppercase tracking-wide">
                      {common.linkLabel}
                    </span>
                    <span className="font-bold text-primary text-sm break-all w-full text-left sm:w-auto sm:text-right">
                      {getRedeemUrl(couponReady.code)}
                    </span>
                  </div>
                </div>
                <div className="space-y-3">
                  <Button fullWidth variant="secondary" onClick={handleCopyCouponInfo}>
                    {copied ? common.copied : common.copyAll}
                  </Button>
                  <Button fullWidth variant="secondary" onClick={handleEmailCouponInfo}>
                    {t.couponEmailInfo}
                  </Button>
                </div>
              </Card>
            </>
          )}

          {error && !couponReady && (
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
