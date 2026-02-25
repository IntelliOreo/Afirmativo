"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { NavHeader } from "@components/NavHeader";
import { Footer } from "@components/Footer";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import { Input } from "@components/Input";
import { Alert } from "@components/Alert";

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "";

export default function PayPage() {
  const router = useRouter();
  const [lang, setLang] = useState<"es" | "en">("es");
  const [coupon, setCoupon] = useState("");
  const [couponError, setCouponError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleCouponSubmit() {
    if (!coupon.trim()) return;

    // TEMP: bypass API validation — any non-empty input proceeds to session page.
    // TODO: remove this block and uncomment the real API call before launch.
    router.push("/session/TEST-0001");
    return;

    // --- real validation (disabled until backend is ready) ---
    // setLoading(true);
    // setCouponError("");
    // try {
    //   const res = await fetch(`${API_URL}/api/v1/coupon/validate`, {
    //     method: "POST",
    //     headers: { "Content-Type": "application/json" },
    //     body: JSON.stringify({ code: coupon.trim() }),
    //   });
    //   const data = await res.json();
    //   if (res.ok && data.sessionCode) {
    //     router.push(`/session/${data.sessionCode}`);
    //   } else {
    //     setCouponError(
    //       lang === "es"
    //         ? "Cupón inválido o ya utilizado. / Invalid or already used coupon."
    //         : "Invalid or already used coupon. / Cupón inválido o ya utilizado."
    //     );
    //   }
    // } catch {
    //   setCouponError(
    //     lang === "es"
    //       ? "Error de conexión. Intente de nuevo. / Connection error. Please try again."
    //       : "Connection error. Please try again. / Error de conexión."
    //   );
    // } finally {
    //   setLoading(false);
    // }
  }

  async function handleStripeCheckout() {
    setLoading(true);
    try {
      const res = await fetch(`${API_URL}/api/v1/payment/checkout`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
      });
      const data = await res.json();
      if (data.url) {
        window.location.href = data.url;
      }
    } catch {
      // TODO: surface error to user
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
        <div className="max-w-lg mx-auto px-4 py-12">
          <h1 className="text-3xl font-bold text-primary-dark mb-2">
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
                labelEs={lang === "en" ? "Cupón" : undefined}
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
