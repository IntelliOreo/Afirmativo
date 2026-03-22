"use client";

import { Suspense, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import { Input } from "@components/Input";
import { Alert } from "@components/Alert";
import { api } from "@/lib/api";
import { withLang } from "@/lib/language";
import { writePin } from "@/lib/storage/sessionPinStore";
import { useLanguage } from "@/lib/useLanguage";
import { getCommonMessages } from "@/messages/commonMessages";
import { getPayMessages } from "@/messages/payMessages";

function PayPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const requestedLang = searchParams.get("lang");
  const { lang, setLang } = useLanguage({ requestedLang });
  const t = getPayMessages(lang);
  const common = getCommonMessages(lang);
  const [coupon, setCoupon] = useState("");
  const [couponError, setCouponError] = useState("");
  const [checkoutError, setCheckoutError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleCouponSubmit() {
    if (!coupon.trim()) return;

    setLoading(true);
    setCouponError("");
    try {
      const { ok, data } = await api<{ valid?: boolean; session_code?: string; pin?: string }>("/api/coupon/validate", {
        method: "POST",
        body: { code: coupon.trim() },
      });

      if (ok && data?.valid && data.session_code) {
        writePin(data.session_code, data.pin ?? "");
        router.push(withLang(`/session/${data.session_code}`, lang));
      } else {
        setCouponError(t.couponInvalid);
      }
    } catch {
      setCouponError(t.couponNetworkError);
    } finally {
      setLoading(false);
    }
  }

  async function handleStripeCheckout() {
    setLoading(true);
    setCheckoutError("");

    try {
      const { ok, data } = await api<{ url?: string }>("/api/payment/checkout", {
        method: "POST",
        body: { lang },
      });

      if (ok && data?.url) {
        window.location.href = data.url;
        return;
      }

      setCheckoutError(t.checkoutFailed);
    } catch {
      setCheckoutError(t.checkoutNetworkError);
    } finally {
      setLoading(false);
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
          <h1 className="text-2xl sm:text-3xl font-bold text-primary-dark mb-2">
            {t.heading}
          </h1>
          <p className="text-primary-darkest mb-8">
            {t.intro}
          </p>

          {/* Coupon section */}
          <Card className="mb-6">
            <h2 className="text-xl font-bold text-primary-dark mb-4">
              {t.couponHeading}
            </h2>
            <form onSubmit={(e) => { e.preventDefault(); handleCouponSubmit(); }} className="space-y-4">
              <Input
                label={t.couponLabel}
                placeholder="EJEMPLO-1234"
                value={coupon}
                onChange={(e) => setCoupon(e.target.value)}
                error={couponError}
                autoCapitalize="characters"
                autoCorrect="off"
              />
              <Button
                type="submit"
                fullWidth
                disabled={loading || !coupon.trim()}
              >
                {loading ? common.verifying : t.couponButton}
              </Button>
            </form>
          </Card>

          {/* Divider */}
          <div className="flex items-center gap-4 mb-6">
            <div className="flex-1 border-t border-base-lighter" />
            <span className="text-sm text-gray-500">
              {t.divider}
            </span>
            <div className="flex-1 border-t border-base-lighter" />
          </div>

          {/* Stripe section */}
          <Card>
            <h2 className="text-xl font-bold text-primary-dark mb-2">
              {t.onlinePaymentHeading}
            </h2>
            <p className="text-sm text-gray-600 mb-4">
              {t.onlinePaymentBody}
            </p>
            <Button fullWidth disabled={loading} onClick={handleStripeCheckout}>
              {t.payByCard}
            </Button>
            {checkoutError && (
              <Alert variant="error" className="mt-4">
                {checkoutError}
              </Alert>
            )}
          </Card>

          {loading && (
            <Alert variant="info" className="mt-4">
              {t.pleaseWait}
            </Alert>
          )}
        </div>
      </main>

      <Footer />
    </div>
  );
}

export default function PayPage() {
  return (
    <Suspense fallback={null}>
      <PayPageContent />
    </Suspense>
  );
}
