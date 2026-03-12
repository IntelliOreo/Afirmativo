import type { Lang } from "@/lib/language";

type PayCopy = {
  heading: string;
  intro: string;
  couponHeading: string;
  couponLabel: string;
  couponButton: string;
  couponInvalid: string;
  couponNetworkError: string;
  divider: string;
  onlinePaymentHeading: string;
  onlinePaymentBody: string;
  payByCard: string;
  cardNotAvailable: string;
  checkoutFailed: string;
  checkoutNetworkError: string;
  pleaseWait: string;
};

const PAY_MESSAGES = {
  en: {
    heading: "Access",
    intro: "Enter a coupon or pay online to get started.",
    couponHeading: "Coupon code",
    couponLabel: "Coupon",
    couponButton: "Apply coupon",
    couponInvalid: "Invalid or already used coupon.",
    couponNetworkError: "Connection error. Please try again.",
    divider: "or",
    onlinePaymentHeading: "Pay online",
    onlinePaymentBody: "Secure payment by credit or debit card.",
    payByCard: "Pay by card",
    cardNotAvailable: "Card payment is not available yet. Please use a coupon to continue.",
    checkoutFailed: "Could not start checkout. Please try again.",
    checkoutNetworkError: "Connection error while starting checkout. Please try again.",
    pleaseWait: "Please wait...",
  },
  es: {
    heading: "Acceso",
    intro: "Ingrese un cupon o pague en linea para comenzar.",
    couponHeading: "Codigo de cupon",
    couponLabel: "Cupon",
    couponButton: "Aplicar cupon",
    couponInvalid: "Cupon invalido o ya utilizado.",
    couponNetworkError: "Error de conexion. Intente de nuevo.",
    divider: "o",
    onlinePaymentHeading: "Pago en linea",
    onlinePaymentBody: "Pago seguro con tarjeta de credito o debito.",
    payByCard: "Pagar con tarjeta",
    cardNotAvailable: "El pago con tarjeta no esta disponible aun. Use un cupon para continuar.",
    checkoutFailed: "No se pudo iniciar el pago. Intente nuevamente.",
    checkoutNetworkError: "Error de conexion al iniciar el pago. Intente nuevamente.",
    pleaseWait: "Por favor espere...",
  },
} as const satisfies Record<Lang, PayCopy>;

export function getPayMessages(lang: Lang) {
  return PAY_MESSAGES[lang];
}

export { PAY_MESSAGES };
