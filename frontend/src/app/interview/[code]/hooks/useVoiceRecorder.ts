"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import { log } from "@/lib/logger";
import {
  VOICE_MAX_SECONDS,
  VOICE_MIME_CANDIDATES,
  VOICE_WARNING_SECONDS,
} from "../constants";
import type { Lang, VoiceRecorderState } from "../types";

interface UseVoiceRecorderParams {
  lang: Lang;
  isActive: boolean;
}

interface UseVoiceRecorderResult {
  voiceRecorderState: VoiceRecorderState;
  voiceDurationSeconds: number;
  voiceWarningSeconds: number | null;
  voiceBlob: Blob | null;
  voicePreviewUrl: string | null;
  isVoicePreviewPlaying: boolean;
  voiceError: string;
  voiceInfo: string;
  isRecordingActive: boolean;
  isRecordingPaused: boolean;
  startVoiceRecording: () => Promise<void>;
  completeVoiceRecording: () => void;
  discardVoiceRecording: () => void;
  toggleVoicePreviewPlayback: () => Promise<void>;
  sendVoiceRecording: (sessionCode: string) => Promise<string | null>;
  setVoiceErrorMessage: (message: string) => void;
}

const voiceAPIURL =
  (process.env.NEXT_PUBLIC_VOICE_API_URL || process.env.VOICE_API_URL || "").trim().replace(/\/+$/, "");

interface DeepgramTokenResponse {
  accessToken: string;
  tokenType: string;
  expiresIn: number;
  provider?: string;
  model?: string;
  error?: string;
  code?: string;
}

class VoiceTranscriptionHTTPError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

function languageCode(lang: Lang): "en" | "es" {
  return lang === "en" ? "en" : "es";
}

function extractTranscript(payload: unknown): string {
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

export function useVoiceRecorder({ lang, isActive }: UseVoiceRecorderParams): UseVoiceRecorderResult {
  const [voiceRecorderState, setVoiceRecorderState] = useState<VoiceRecorderState>("idle");
  const [voiceDurationSeconds, setVoiceDurationSeconds] = useState(0);
  const [voiceWarningSeconds, setVoiceWarningSeconds] = useState<number | null>(null);
  const [voiceBlob, setVoiceBlob] = useState<Blob | null>(null);
  const [voicePreviewBlob, setVoicePreviewBlob] = useState<Blob | null>(null);
  const [voicePreviewUrl, setVoicePreviewUrl] = useState<string | null>(null);
  const [isVoicePreviewPlaying, setIsVoicePreviewPlaying] = useState(false);
  const [voiceError, setVoiceError] = useState("");
  const [voiceInfo, setVoiceInfo] = useState("");

  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const mediaStreamRef = useRef<MediaStream | null>(null);
  const previewAudioRef = useRef<HTMLAudioElement | null>(null);
  const voiceChunksRef = useRef<BlobPart[]>([]);
  const voiceTickerRef = useRef<number | null>(null);
  const voiceStartMsRef = useRef<number>(0);
  const voiceElapsedMsRef = useRef<number>(0);
  const voiceDurationRef = useRef(voiceDurationSeconds);
  voiceDurationRef.current = voiceDurationSeconds;
  const langRef = useRef(lang);
  langRef.current = lang;

  const clearVoiceTicker = useCallback(() => {
    if (voiceTickerRef.current !== null) {
      window.clearInterval(voiceTickerRef.current);
      voiceTickerRef.current = null;
    }
  }, []);

  const stopVoiceStreamTracks = useCallback(() => {
    if (!mediaStreamRef.current) return;
    mediaStreamRef.current.getTracks().forEach((track) => track.stop());
    mediaStreamRef.current = null;
  }, []);

  const discardVoiceRecording = useCallback(() => {
    const recorder = mediaRecorderRef.current;
    if (recorder && recorder.state !== "inactive") {
      recorder.ondataavailable = null;
      recorder.onstop = null;
      recorder.onerror = null;
      try {
        recorder.stop();
      } catch {
        // Ignore recorder stop errors during cleanup.
      }
    }

    mediaRecorderRef.current = null;
    clearVoiceTicker();
    stopVoiceStreamTracks();
    voiceChunksRef.current = [];
    voiceStartMsRef.current = 0;
    voiceElapsedMsRef.current = 0;
    voiceDurationRef.current = 0;
    setVoiceRecorderState("idle");
    setVoiceDurationSeconds(0);
    setVoiceWarningSeconds(null);
    setVoiceBlob(null);
    setVoicePreviewBlob(null);
    setVoicePreviewUrl(null);
    setIsVoicePreviewPlaying(false);
    setVoiceError("");
    setVoiceInfo("");
    if (previewAudioRef.current) {
      previewAudioRef.current.pause();
      previewAudioRef.current.currentTime = 0;
      previewAudioRef.current.onplay = null;
      previewAudioRef.current.onpause = null;
      previewAudioRef.current.onended = null;
      previewAudioRef.current = null;
    }
  }, [clearVoiceTicker, stopVoiceStreamTracks]);

  const completeVoiceRecording = useCallback(() => {
    const recorder = mediaRecorderRef.current;
    if (!recorder || recorder.state === "inactive") return;
    if (recorder.state === "recording" && voiceStartMsRef.current > 0) {
      voiceElapsedMsRef.current += Date.now() - voiceStartMsRef.current;
      voiceStartMsRef.current = 0;
    }
    clearVoiceTicker();
    try {
      recorder.stop();
    } catch {
      // Ignore recorder stop errors.
    }
  }, [clearVoiceTicker]);

  const startVoiceTicker = useCallback(() => {
    clearVoiceTicker();
    voiceStartMsRef.current = Date.now();
    voiceTickerRef.current = window.setInterval(() => {
      if (voiceStartMsRef.current <= 0) return;
      const elapsedMs = voiceElapsedMsRef.current + (Date.now() - voiceStartMsRef.current);
      const elapsedSeconds = Math.min(VOICE_MAX_SECONDS, Math.floor(elapsedMs / 1000));
      if (elapsedSeconds !== voiceDurationRef.current) {
        voiceDurationRef.current = elapsedSeconds;
        setVoiceDurationSeconds(elapsedSeconds);
        if (VOICE_WARNING_SECONDS.includes(elapsedSeconds as typeof VOICE_WARNING_SECONDS[number])) {
          setVoiceWarningSeconds(elapsedSeconds);
        }
      }
      if (elapsedSeconds >= VOICE_MAX_SECONDS) {
        voiceElapsedMsRef.current = VOICE_MAX_SECONDS * 1000;
        setVoiceInfo(
          langRef.current === "es"
            ? "Se alcanzó el límite de 3 minutos y la grabación se detuvo."
            : "The 3-minute limit was reached and recording stopped.",
        );
        completeVoiceRecording();
      }
    }, 250);
  }, [clearVoiceTicker, completeVoiceRecording]);

  const pauseVoiceRecording = useCallback(() => {
    const recorder = mediaRecorderRef.current;
    if (!recorder || recorder.state !== "recording") return;
    if (voiceStartMsRef.current > 0) {
      voiceElapsedMsRef.current += Date.now() - voiceStartMsRef.current;
      voiceStartMsRef.current = 0;
    }
    clearVoiceTicker();
    try {
      recorder.requestData();
    } catch {
      // Ignore requestData failures; pause still proceeds.
    }
    recorder.pause();
    setVoiceRecorderState("paused");
  }, [clearVoiceTicker]);

  const resumeVoiceRecording = useCallback(() => {
    const recorder = mediaRecorderRef.current;
    if (!recorder || recorder.state !== "paused") return;
    if (previewAudioRef.current) {
      previewAudioRef.current.pause();
      previewAudioRef.current.currentTime = 0;
    }
    setIsVoicePreviewPlaying(false);
    recorder.resume();
    setVoiceRecorderState("recording");
    startVoiceTicker();
  }, [startVoiceTicker]);

  const startVoiceRecording = useCallback(async () => {
    if (!isActive) return;
    if (voiceRecorderState === "recording") {
      pauseVoiceRecording();
      return;
    }
    if (voiceRecorderState === "paused") {
      resumeVoiceRecording();
      return;
    }
    if (voiceRecorderState !== "idle") return;
    if (typeof window === "undefined") return;

    if (!window.isSecureContext && window.location.hostname !== "localhost") {
      setVoiceError(
        langRef.current === "es"
          ? "La grabación de voz requiere HTTPS o localhost."
          : "Voice recording requires HTTPS or localhost.",
      );
      return;
    }

    if (
      typeof navigator === "undefined"
      || !navigator.mediaDevices?.getUserMedia
      || typeof MediaRecorder === "undefined"
    ) {
      setVoiceError(
        langRef.current === "es"
          ? "Este navegador no soporta grabación de audio."
          : "This browser does not support audio recording.",
      );
      return;
    }

    discardVoiceRecording();
    setVoiceWarningSeconds(null);

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      mediaStreamRef.current = stream;

      const mimeType = VOICE_MIME_CANDIDATES.find((candidate) => MediaRecorder.isTypeSupported(candidate));
      const recorder = mimeType
        ? new MediaRecorder(stream, { mimeType })
        : new MediaRecorder(stream);

      recorder.ondataavailable = (event: BlobEvent) => {
        if (event.data.size > 0) {
          voiceChunksRef.current.push(event.data);
          if (recorder.state === "paused") {
            const previewBlob = new Blob(voiceChunksRef.current, { type: recorder.mimeType || "audio/webm" });
            if (previewBlob.size > 0) {
              setVoicePreviewBlob(previewBlob);
            }
          }
        }
      };

      recorder.onerror = () => {
        mediaRecorderRef.current = null;
        clearVoiceTicker();
        stopVoiceStreamTracks();
        voiceStartMsRef.current = 0;
        voiceElapsedMsRef.current = 0;
        setVoiceRecorderState("idle");
        setVoiceError(
          langRef.current === "es"
            ? "No se pudo completar la grabación."
            : "Unable to complete recording.",
        );
      };

      recorder.onstop = () => {
        mediaRecorderRef.current = null;
        clearVoiceTicker();
        stopVoiceStreamTracks();
        voiceStartMsRef.current = 0;

        const elapsed = Math.min(
          VOICE_MAX_SECONDS,
          Math.max(voiceDurationRef.current, Math.floor(voiceElapsedMsRef.current / 1000)),
        );
        setVoiceDurationSeconds(elapsed);

        const chunks = voiceChunksRef.current;
        voiceChunksRef.current = [];
        if (chunks.length === 0) {
          setVoiceRecorderState("idle");
          setVoiceBlob(null);
          setVoiceError(
            langRef.current === "es"
              ? "No se detectó audio. Intente grabar de nuevo."
              : "No audio detected. Please record again.",
          );
          return;
        }

        const blob = new Blob(chunks, { type: recorder.mimeType || "audio/webm" });
        if (blob.size === 0) {
          setVoiceRecorderState("idle");
          setVoiceBlob(null);
          setVoiceError(
            langRef.current === "es"
              ? "No se detectó audio. Intente grabar de nuevo."
              : "No audio detected. Please record again.",
          );
          return;
        }

        setVoiceBlob(blob);
        setVoicePreviewBlob(blob);
        setVoiceRecorderState("stopped");
      };

      mediaRecorderRef.current = recorder;
      voiceChunksRef.current = [];
      voiceElapsedMsRef.current = 0;
      voiceStartMsRef.current = 0;
      voiceDurationRef.current = 0;
      setVoiceDurationSeconds(0);
      setVoiceBlob(null);
      setVoicePreviewBlob(null);
      setVoiceError("");
      setVoiceInfo("");
      setVoiceRecorderState("recording");
      recorder.start(250);
      startVoiceTicker();
    } catch {
      mediaRecorderRef.current = null;
      clearVoiceTicker();
      stopVoiceStreamTracks();
      voiceStartMsRef.current = 0;
      voiceElapsedMsRef.current = 0;
      setVoiceRecorderState("idle");
      setVoiceError(
        langRef.current === "es"
          ? "No se pudo acceder al micrófono."
          : "Unable to access microphone.",
      );
    }
  }, [
    clearVoiceTicker,
    discardVoiceRecording,
    isActive,
    pauseVoiceRecording,
    resumeVoiceRecording,
    startVoiceTicker,
    stopVoiceStreamTracks,
    voiceRecorderState,
  ]);

  const toggleVoicePreviewPlayback = useCallback(async () => {
    if (!previewAudioRef.current || !voicePreviewUrl) return;
    const audio = previewAudioRef.current;
    if (!audio.paused) {
      audio.pause();
      return;
    }
    if (audio.ended) {
      audio.currentTime = 0;
    }
    try {
      await audio.play();
    } catch {
      setVoiceError(
        langRef.current === "es"
          ? "No se pudo reproducir el audio grabado."
          : "Unable to play the recorded audio.",
      );
    }
  }, [voicePreviewUrl]);

  const sendVoiceRecording = useCallback(async (sessionCode: string): Promise<string | null> => {
    if (!isActive || voiceRecorderState !== "stopped" || !voiceBlob) return null;
    const audioBlob = voiceBlob;

    const trimmedSessionCode = sessionCode.trim();
      log.debug("voice send requested", {
        phase: "send_start",
        session_code: trimmedSessionCode,
        recorder_state: voiceRecorderState,
        blob_size_bytes: audioBlob.size,
        blob_type: audioBlob.type || "application/octet-stream",
        language: languageCode(langRef.current),
      });

    if (!trimmedSessionCode) {
      setVoiceError(
        langRef.current === "es"
          ? "Falta el código de sesión."
          : "Session code is missing.",
      );
      return null;
    }

    if (!voiceAPIURL) {
      setVoiceError(
        langRef.current === "es"
          ? "VOICE_API_URL no está configurado."
          : "VOICE_API_URL is not configured.",
      );
      return null;
    }

    if (previewAudioRef.current) {
      previewAudioRef.current.pause();
      previewAudioRef.current.currentTime = 0;
    }
    setIsVoicePreviewPlaying(false);
    setVoiceRecorderState("sending");
    setVoiceError("");
    setVoiceInfo(
      langRef.current === "es"
        ? "Solicitando token temporal..."
        : "Requesting short-lived token...",
    );

    async function mintToken() {
      const requestPayload = { sessionCode: trimmedSessionCode };
      log.debug("voice token mint request", {
        phase: "token_request",
        backend_path: "/api/deepgram/token",
      });
      const { ok, status, data } = await api<DeepgramTokenResponse>("/api/deepgram/token", {
        method: "POST",
        body: requestPayload,
        credentials: "include",
      });
      log.debug("voice token mint response", {
        phase: "token_response",
        ok,
        status,
        provider: data?.provider ?? "",
        model: data?.model ?? "",
        has_access_token: !!data?.accessToken,
      });
      if (!ok || !data?.accessToken) {
        log.warn("voice token mint failed", {
          phase: "token_response",
          status,
          error: data?.error ?? "missing_access_token",
          code: data?.code ?? "",
        });
        throw new Error(
          data?.error
            || (status === 401
              ? (langRef.current === "es" ? "Sesión no autorizada." : "Session is not authorized.")
              : (langRef.current === "es" ? "No se pudo obtener token de voz." : "Failed to get voice token.")),
        );
      }
      return data;
    }

    async function transcribe(accessToken: string, tokenType?: string, model?: string) {
      const query = new URLSearchParams({
        mip_opt_out: "true",
        language: languageCode(langRef.current),
      });
      if (model && model.trim()) {
        query.set("model", model.trim());
      }
      const authScheme = tokenType && tokenType.trim()
        ? tokenType.trim()
        : "Bearer";
      const listenURL = `${voiceAPIURL}/v1/listen?${query.toString()}`;
      const requestHeaders = {
        Authorization: `${authScheme} ${accessToken}`,
        "Content-Type": audioBlob.type || "application/octet-stream",
      };
      log.debug("voice transcription request", {
        phase: "transcription_request",
        url: listenURL,
        method: "POST",
        blob_size_bytes: audioBlob.size,
      });

      const response = await fetch(listenURL, {
        method: "POST",
        headers: requestHeaders,
        body: audioBlob,
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
            || (langRef.current === "es"
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
          langRef.current === "es"
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

    try {
      const token = await mintToken();
      setVoiceInfo(
        langRef.current === "es"
          ? "Enviando audio para transcripción..."
          : "Uploading audio for transcription...",
      );

      let transcript: string;
      try {
        transcript = await transcribe(token.accessToken, token.tokenType, token.model);
      } catch (err) {
        if (
          err instanceof VoiceTranscriptionHTTPError
          && (err.status === 401 || err.status === 403)
        ) {
          log.warn("voice transcription retrying after auth status", {
            phase: "transcription_retry",
            status: err.status,
          });
          setVoiceInfo(
            langRef.current === "es"
              ? "Token expirado. Reintentando..."
              : "Token expired. Retrying...",
          );
          const retryToken = await mintToken();
          transcript = await transcribe(retryToken.accessToken, retryToken.tokenType, retryToken.model);
        } else {
          throw err;
        }
      }

      setVoiceRecorderState("stopped");
      setVoiceInfo(
        langRef.current === "es"
          ? "Transcripción lista. Enviando respuesta..."
          : "Transcript ready. Submitting answer...",
      );
      log.info("voice transcription completed", {
        phase: "send_done",
        transcript_length: transcript.length,
      });
      return transcript;
    } catch (err) {
      setVoiceRecorderState("stopped");
      setVoiceInfo("");
      log.error("voice send/transcription failed", {
        phase: "send_error",
        error: err instanceof Error ? err.message : "unknown_error",
      });
      setVoiceError(
        err instanceof Error
          ? err.message
          : (langRef.current === "es" ? "Error de transcripción." : "Transcription error."),
      );
      return null;
    }
  }, [isActive, voiceBlob, voiceRecorderState]);

  useEffect(() => {
    if (!voicePreviewBlob) {
      setVoicePreviewUrl(null);
      setIsVoicePreviewPlaying(false);
      if (previewAudioRef.current) {
        previewAudioRef.current.pause();
        previewAudioRef.current.currentTime = 0;
        previewAudioRef.current.onplay = null;
        previewAudioRef.current.onpause = null;
        previewAudioRef.current.onended = null;
        previewAudioRef.current = null;
      }
      return;
    }

    const url = URL.createObjectURL(voicePreviewBlob);
    setVoicePreviewUrl(url);
    setIsVoicePreviewPlaying(false);
    if (previewAudioRef.current) {
      previewAudioRef.current.pause();
      previewAudioRef.current.currentTime = 0;
      previewAudioRef.current.onplay = null;
      previewAudioRef.current.onpause = null;
      previewAudioRef.current.onended = null;
    }
    const previewAudio = new Audio(url);
    previewAudio.onplay = () => setIsVoicePreviewPlaying(true);
    previewAudio.onpause = () => setIsVoicePreviewPlaying(false);
    previewAudio.onended = () => setIsVoicePreviewPlaying(false);
    previewAudioRef.current = previewAudio;
    return () => {
      previewAudio.pause();
      previewAudio.currentTime = 0;
      previewAudio.onplay = null;
      previewAudio.onpause = null;
      previewAudio.onended = null;
      if (previewAudioRef.current === previewAudio) {
        previewAudioRef.current = null;
      }
      URL.revokeObjectURL(url);
    };
  }, [voicePreviewBlob]);

  useEffect(() => {
    if (isActive) return;
    if (voiceRecorderState !== "idle") {
      discardVoiceRecording();
    }
  }, [discardVoiceRecording, isActive, voiceRecorderState]);

  useEffect(() => {
    return () => {
      const recorder = mediaRecorderRef.current;
      if (recorder && recorder.state !== "inactive") {
        recorder.ondataavailable = null;
        recorder.onstop = null;
        recorder.onerror = null;
        try {
          recorder.stop();
        } catch {
          // Ignore recorder stop errors during unmount.
        }
      }
      mediaRecorderRef.current = null;
      if (voiceTickerRef.current !== null) {
        window.clearInterval(voiceTickerRef.current);
      }
      voiceTickerRef.current = null;
      if (mediaStreamRef.current) {
        mediaStreamRef.current.getTracks().forEach((track) => track.stop());
      }
      mediaStreamRef.current = null;
      if (previewAudioRef.current) {
        previewAudioRef.current.pause();
        previewAudioRef.current.onplay = null;
        previewAudioRef.current.onpause = null;
        previewAudioRef.current.onended = null;
        previewAudioRef.current = null;
      }
    };
  }, []);

  return {
    voiceRecorderState,
    voiceDurationSeconds,
    voiceWarningSeconds,
    voiceBlob,
    voicePreviewUrl,
    isVoicePreviewPlaying,
    voiceError,
    voiceInfo,
    isRecordingActive: voiceRecorderState === "recording",
    isRecordingPaused: voiceRecorderState === "paused",
    startVoiceRecording,
    completeVoiceRecording,
    discardVoiceRecording,
    toggleVoicePreviewPlayback,
    sendVoiceRecording,
    setVoiceErrorMessage: setVoiceError,
  };
}
