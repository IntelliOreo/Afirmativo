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

function PayPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const requestedLang = searchParams.get("lang");
  const { lang, setLang } = useLanguage({ requestedLang });
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
        setCouponError(
          lang === "es"
            ? "Cupón inválido o ya utilizado. / Invalid or already used coupon."
            : "Invalid or already used coupon. / Cupón inválido o ya utilizado.",
        );
      }
    } catch {
      setCouponError(
        lang === "es"
          ? "Error de conexión. Intente de nuevo. / Connection error. Please try again."
          : "Connection error. Please try again. / Error de conexión.",
      );
    } finally {
      setLoading(false);
    }
  }

  async function handleStripeCheckout() {
    setLoading(true);
    setCheckoutError("");

    try {
      const { ok, status, data } = await api<{ url?: string; error?: string; code?: string }>("/api/payment/checkout", {
        method: "POST",
      });

      if (ok && data?.url) {
        window.location.href = data.url;
        return;
      }

      if (status === 501 || data?.code === "PAYMENT_NOT_IMPLEMENTED") {
        setCheckoutError(
          lang === "es"
            ? "El pago con tarjeta no está disponible aún. Use un cupón para continuar."
            : "Card payment is not available yet. Please use a coupon to continue.",
        );
        return;
      }

      setCheckoutError(
        data?.error
          || (lang === "es"
            ? "No se pudo iniciar el pago. Intente nuevamente."
            : "Could not start checkout. Please try again."),
      );
    } catch {
      setCheckoutError(
        lang === "es"
          ? "Error de conexión al iniciar el pago. Intente nuevamente."
          : "Connection error while starting checkout. Please try again.",
      );
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
            {lang === "es" ? "Acceso" : "Access"}
          </h1>
          <p className="text-primary-darkest mb-8">
            {lang === "es"
              ? "Ingrese un cupón o pague en línea para comenzar."
              : "Enter a coupon or pay online to get started."}
          </p>

          {/* Coupon section */}
          <Card className="mb-6">
            <h2 className="text-xl font-bold text-primary-dark mb-4">
              {lang === "es" ? "Código de cupón" : "Coupon code"}
            </h2>
            <form onSubmit={(e) => { e.preventDefault(); handleCouponSubmit(); }} className="space-y-4">
              <Input
                label={lang === "es" ? "Cupón" : "Coupon"}
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
                {loading
                  ? lang === "es"
                    ? "Verificando..."
                    : "Verifying..."
                  : lang === "es"
                    ? "Aplicar cupón"
                    : "Apply coupon"}
              </Button>
            </form>
          </Card>

          {/* Divider */}
          <div className="flex items-center gap-4 mb-6">
            <div className="flex-1 border-t border-base-lighter" />
            <span className="text-sm text-gray-500">
              {lang === "es" ? "o" : "or"}
            </span>
            <div className="flex-1 border-t border-base-lighter" />
          </div>

          {/* Stripe section */}
          <Card>
            <h2 className="text-xl font-bold text-primary-dark mb-2">
              {lang === "es" ? "Pago en línea" : "Pay online"}
            </h2>
            <p className="text-sm text-gray-600 mb-4">
              {lang === "es"
                ? "Pago seguro con tarjeta de crédito o débito."
                : "Secure payment by credit or debit card."}
            </p>
            <Button fullWidth disabled={loading} onClick={handleStripeCheckout}>
              {lang === "es" ? "Pagar con tarjeta" : "Pay by card"}
            </Button>
            {checkoutError && (
              <Alert variant="error" className="mt-4">
                {checkoutError}
              </Alert>
            )}
          </Card>

          {loading && (
            <Alert variant="info" className="mt-4">
              {lang === "es" ? "Por favor espere..." : "Please wait..."}
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
