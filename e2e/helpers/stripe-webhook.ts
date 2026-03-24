import crypto from "node:crypto";

export function signStripeWebhook(payload: string, secret: string, timestamp = Math.floor(Date.now() / 1000)): string {
  const signature = crypto
    .createHmac("sha256", secret)
    .update(`${timestamp}.${payload}`)
    .digest("hex");

  return `t=${timestamp},v1=${signature}`;
}
