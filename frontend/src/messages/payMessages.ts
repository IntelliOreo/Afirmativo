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
  checkoutFailed: string;
  checkoutNetworkError: string;
  pleaseWait: string;
  successHeading: string;
  successBody: string;
  successPending: string;
  missingCheckoutSession: string;
  paymentStatusFailed: string;
  paymentStatusNetworkError: string;
  paymentStatusTimeout: string;
  paymentRevealExpired: string;
  returnToPay: string;
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
    checkoutFailed: "Could not start checkout. Please try again.",
    checkoutNetworkError: "Connection error while starting checkout. Please try again.",
    pleaseWait: "Please wait...",
    successHeading: "Finishing payment",
    successBody: "We are confirming your payment and preparing your session details.",
    successPending: "Almost done. Please keep this page open.",
    missingCheckoutSession: "Missing checkout session. Please return to the payment page and try again.",
    paymentStatusFailed: "We could not finish your payment handoff. Please return to the payment page and try again.",
    paymentStatusNetworkError: "Connection error while checking payment status. Please try again.",
    paymentStatusTimeout: "Payment confirmation is taking longer than expected. Please try again from the payment page.",
    paymentRevealExpired: "Your payment was confirmed, but the PIN handoff expired. Please contact support or try again from the payment page.",
    returnToPay: "Back to payment",
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
    checkoutFailed: "No se pudo iniciar el pago. Intente nuevamente.",
    checkoutNetworkError: "Error de conexion al iniciar el pago. Intente nuevamente.",
    pleaseWait: "Por favor espere...",
    successHeading: "Finalizando pago",
    successBody: "Estamos confirmando su pago y preparando los datos de su sesion.",
    successPending: "Ya casi terminamos. Por favor mantenga esta pagina abierta.",
    missingCheckoutSession: "Falta la sesion de pago. Regrese a la pagina de pago e intente nuevamente.",
    paymentStatusFailed: "No pudimos completar la entrega de su pago. Regrese a la pagina de pago e intente nuevamente.",
    paymentStatusNetworkError: "Error de conexion al verificar el estado del pago. Intente nuevamente.",
    paymentStatusTimeout: "La confirmacion del pago esta tardando mas de lo esperado. Intente nuevamente desde la pagina de pago.",
    paymentRevealExpired: "Su pago fue confirmado, pero la entrega del PIN expiro. Contacte soporte o intente nuevamente desde la pagina de pago.",
    returnToPay: "Volver al pago",
  },
} as const satisfies Record<Lang, PayCopy>;

export function getPayMessages(lang: Lang) {
  return PAY_MESSAGES[lang];
}

export { PAY_MESSAGES };
