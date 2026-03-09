import { api } from "@/lib/api";
import { log } from "@/lib/logger";
import type { Lang } from "@/lib/language";

const voiceAPIURL =
  (process.env.NEXT_PUBLIC_VOICE_API_URL || process.env.VOICE_API_URL || "").trim().replace(/\/+$/, "");

export interface VoiceTokenResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
  provider?: string;
  model?: string;
  error?: string;
  code?: string;
}

export class VoiceTranscriptionHTTPError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

function languageCode(lang: Lang): "en" | "es" {
  return lang === "en" ? "en" : "es";
}

export function extractTranscript(payload: unknown): string {
  if (!payload || typeof payload !== "object") return "";

  const root = payload as Record<string, unknown>;
  if (typeof root.transcript === "string" && root.transcript.trim()) {
    return root.transcript.trim();
  }

  const results = root.results;
  if (!results || typeof results !== "object") return "";
  const channels = (results as { channels?: unknown }).channels;
  if (!Array.isArray(channels) || channels.length === 0) return "";
  const first = channels[0];
  if (!first || typeof first !== "object") return "";
  const alternatives = (first as { alternatives?: unknown }).alternatives;
  if (!Array.isArray(alternatives) || alternatives.length === 0) return "";
  const transcript = (alternatives[0] as { transcript?: unknown }).transcript;
  return typeof transcript === "string" ? transcript.trim() : "";
}

async function mintToken(sessionCode: string, lang: Lang): Promise<VoiceTokenResponse> {
  log.debug("voice token mint request", {
    phase: "token_request",
    backend_path: "/api/voice/token",
  });

  const { ok, status, data } = await api<VoiceTokenResponse>("/api/voice/token", {
    method: "POST",
    body: { session_code: sessionCode },
    credentials: "include",
  });

  log.debug("voice token mint response", {
    phase: "token_response",
    ok,
    status,
    provider: data?.provider ?? "",
    model: data?.model ?? "",
    has_access_token: !!data?.access_token,
  });

  if (!ok || !data?.access_token) {
    log.warn("voice token mint failed", {
      phase: "token_response",
      status,
      error: data?.error ?? "missing_access_token",
      code: data?.code ?? "",
    });
    throw new Error(
      data?.error
        || (status === 401
          ? (lang === "es" ? "Sesión no autorizada." : "Session is not authorized.")
          : (lang === "es" ? "No se pudo obtener token de voz." : "Failed to get voice token.")),
    );
  }

  return data;
}

async function requestTranscription(
  blob: Blob,
  lang: Lang,
  accessToken: string,
  tokenType?: string,
  model?: string,
): Promise<string> {
  const query = new URLSearchParams({
    mip_opt_out: "true",
    language: languageCode(lang),
  });
  if (model && model.trim()) {
    query.set("model", model.trim());
  }

  const authScheme = tokenType && tokenType.trim()
    ? tokenType.trim()
    : "Bearer";
  const listenURL = `${voiceAPIURL}/v1/listen?${query.toString()}`;

  log.debug("voice transcription request", {
    phase: "transcription_request",
    url: listenURL,
    method: "POST",
    blob_size_bytes: blob.size,
  });

  const response = await fetch(listenURL, {
    method: "POST",
    headers: {
      Authorization: `${authScheme} ${accessToken}`,
      "Content-Type": blob.type || "application/octet-stream",
    },
    body: blob,
  });

  let payload: unknown = null;
  try {
    payload = await response.json();
  } catch {
    // Non-JSON body from provider.
  }

  log.debug("voice transcription response", {
    phase: "transcription_response",
    status: response.status,
    ok: response.ok,
  });

  if (!response.ok) {
    const errorMessage = (payload as { error?: string } | null)?.error;
    log.warn("voice transcription failed", {
      phase: "transcription_response",
      status: response.status,
      error: errorMessage || "provider_non_2xx",
    });
    throw new VoiceTranscriptionHTTPError(
      response.status,
      errorMessage
        || (lang === "es"
          ? "Falló la transcripción de audio."
          : "Audio transcription failed."),
    );
  }

  const transcript = extractTranscript(payload);
  if (!transcript) {
    log.warn("voice transcription returned empty transcript", {
      phase: "transcription_parse",
    });
    throw new Error(
      lang === "es"
        ? "No se pudo obtener texto de la grabación."
        : "Could not extract text from recording.",
    );
  }

  log.debug("voice transcript extracted", {
    phase: "transcription_parse",
    transcript_length: transcript.length,
  });

  return transcript;
}

export async function transcribeAudio(blob: Blob, lang: Lang, sessionCode: string): Promise<string> {
  const trimmedSessionCode = sessionCode.trim();
  if (!trimmedSessionCode) {
    throw new Error(lang === "es" ? "Falta el código de sesión." : "Session code is missing.");
  }

  if (!voiceAPIURL) {
    throw new Error(lang === "es" ? "VOICE_API_URL no está configurado." : "VOICE_API_URL is not configured.");
  }

  const token = await mintToken(trimmedSessionCode, lang);
  try {
    return await requestTranscription(blob, lang, token.access_token, token.token_type, token.model);
  } catch (err) {
    if (
      err instanceof VoiceTranscriptionHTTPError
      && (err.status === 401 || err.status === 403)
    ) {
      log.warn("voice transcription retrying after auth status", {
        phase: "transcription_retry",
        status: err.status,
      });
      const retryToken = await mintToken(trimmedSessionCode, lang);
      return requestTranscription(blob, lang, retryToken.access_token, retryToken.token_type, retryToken.model);
    }
    throw err;
  }
}
