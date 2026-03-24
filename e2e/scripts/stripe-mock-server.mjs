import http from "node:http";
import crypto from "node:crypto";

const sessions = new Map();

function json(response, statusCode, payload) {
  response.writeHead(statusCode, { "Content-Type": "application/json" });
  response.end(JSON.stringify(payload));
}

function notFound(response) {
  json(response, 404, { error: "Not found" });
}

const server = http.createServer(async (request, response) => {
  if (!request.url) {
    notFound(response);
    return;
  }

  const url = new URL(request.url, "http://127.0.0.1:12111");

  if (request.method === "POST" && url.pathname === "/v1/checkout/sessions") {
    const chunks = [];
    for await (const chunk of request) {
      chunks.push(Buffer.from(chunk));
    }

    const form = new URLSearchParams(Buffer.concat(chunks).toString("utf8"));
    const sessionId = `cs_test_${crypto.randomBytes(6).toString("hex")}`;
    const successUrl = (form.get("success_url") ?? "").replace("{CHECKOUT_SESSION_ID}", sessionId);

    const record = {
      id: sessionId,
      client_reference_id: form.get("client_reference_id") ?? "",
      success_url: successUrl,
      amount_total: Number(form.get("line_items[0][price_data][unit_amount]") ?? "0"),
      currency: form.get("line_items[0][price_data][currency]") ?? "usd",
    };
    sessions.set(sessionId, record);

    json(response, 200, {
      id: sessionId,
      url: successUrl,
    });
    return;
  }

  if (request.method === "GET" && url.pathname.startsWith("/__mock/checkout-sessions/")) {
    const sessionId = url.pathname.split("/").pop() ?? "";
    const record = sessions.get(sessionId);
    if (!record) {
      notFound(response);
      return;
    }
    json(response, 200, record);
    return;
  }

  notFound(response);
});

server.listen(12111, "127.0.0.1");
