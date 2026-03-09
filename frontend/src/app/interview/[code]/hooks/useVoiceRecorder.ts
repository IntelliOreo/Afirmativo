"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { log } from "@/lib/logger";
import type { Lang } from "@/lib/language";
import {
  VOICE_MAX_SECONDS,
  VOICE_MIME_CANDIDATES,
  VOICE_WARNING_SECONDS,
} from "../constants";
import { transcribeAudio } from "../lib/voiceTranscription";
import type { VoiceRecorderState } from "../viewTypes";
import { useVoicePreview } from "./useVoicePreview";
import { useVoiceTicker } from "./useVoiceTicker";

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

export function useVoiceRecorder({ lang, isActive }: UseVoiceRecorderParams): UseVoiceRecorderResult {
  const [voiceRecorderState, setVoiceRecorderState] = useState<VoiceRecorderState>("idle");
  const [voiceBlob, setVoiceBlob] = useState<Blob | null>(null);
  const [voiceError, setVoiceError] = useState("");
  const [voiceInfo, setVoiceInfo] = useState("");

  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const mediaStreamRef = useRef<MediaStream | null>(null);
  const voiceChunksRef = useRef<BlobPart[]>([]);
  const langRef = useRef(lang);
  langRef.current = lang;

  const stopVoiceStreamTracks = useCallback(() => {
    if (!mediaStreamRef.current) return;
    mediaStreamRef.current.getTracks().forEach((track) => track.stop());
    mediaStreamRef.current = null;
  }, []);

  const completeVoiceRecordingRef = useRef<() => void>(() => {});
  const {
    durationSeconds: voiceDurationSeconds,
    warningSeconds: voiceWarningSeconds,
    start: startVoiceTicker,
    pause: pauseVoiceTicker,
    resume: resumeVoiceTicker,
    stop: stopVoiceTicker,
    reset: resetVoiceTicker,
  } = useVoiceTicker({
    maxSeconds: VOICE_MAX_SECONDS,
    warningMilestones: VOICE_WARNING_SECONDS,
    onLimitReached: () => {
      setVoiceInfo(
        langRef.current === "es"
          ? "Se alcanzo el limite de 3 minutos y la grabacion se detuvo."
          : "The 3-minute limit was reached and recording stopped.",
      );
      completeVoiceRecordingRef.current();
    },
  });
  const {
    previewUrl: voicePreviewUrl,
    isPlaying: isVoicePreviewPlaying,
    setPreviewBlob,
    togglePlayback,
    stopPlayback,
    clearPreview,
  } = useVoicePreview();

  const detachRecorder = useCallback((recorder: MediaRecorder | null = mediaRecorderRef.current) => {
    if (!recorder) return;
    recorder.ondataavailable = null;
    recorder.onstop = null;
    recorder.onerror = null;
    if (mediaRecorderRef.current === recorder) {
      mediaRecorderRef.current = null;
    }
  }, []);

  const discardVoiceRecording = useCallback(() => {
    const recorder = mediaRecorderRef.current;
    if (recorder) {
      detachRecorder(recorder);
      if (recorder.state !== "inactive") {
        try {
          recorder.stop();
        } catch {
          // Ignore recorder stop errors during cleanup.
        }
      }
    }

    stopVoiceStreamTracks();
    voiceChunksRef.current = [];
    resetVoiceTicker();
    clearPreview();
    setVoiceBlob(null);
    setVoiceRecorderState("idle");
    setVoiceError("");
    setVoiceInfo("");
  }, [clearPreview, detachRecorder, resetVoiceTicker, stopVoiceStreamTracks]);

  const completeVoiceRecording = useCallback(() => {
    const recorder = mediaRecorderRef.current;
    if (!recorder || recorder.state === "inactive") return;
    stopVoiceTicker();
    try {
      recorder.stop();
    } catch {
      // Ignore recorder stop errors.
    }
  }, [stopVoiceTicker]);
  completeVoiceRecordingRef.current = completeVoiceRecording;

  const pauseVoiceRecording = useCallback(() => {
    const recorder = mediaRecorderRef.current;
    if (!recorder || recorder.state !== "recording") return;
    pauseVoiceTicker();
    try {
      recorder.requestData();
    } catch {
      // Ignore requestData failures; pause still proceeds.
    }
    recorder.pause();
    setVoiceRecorderState("paused");
  }, [pauseVoiceTicker]);

  const resumeVoiceRecording = useCallback(() => {
    const recorder = mediaRecorderRef.current;
    if (!recorder || recorder.state !== "paused") return;
    stopPlayback();
    recorder.resume();
    setVoiceRecorderState("recording");
    resumeVoiceTicker();
  }, [resumeVoiceTicker, stopPlayback]);

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
          ? "La grabacion de voz requiere HTTPS o localhost."
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
          ? "Este navegador no soporta grabacion de audio."
          : "This browser does not support audio recording.",
      );
      return;
    }

    discardVoiceRecording();

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      mediaStreamRef.current = stream;

      const mimeType = VOICE_MIME_CANDIDATES.find((candidate) => MediaRecorder.isTypeSupported(candidate));
      const recorder = mimeType
        ? new MediaRecorder(stream, { mimeType })
        : new MediaRecorder(stream);

      recorder.ondataavailable = (event: BlobEvent) => {
        if (event.data.size <= 0) return;

        voiceChunksRef.current.push(event.data);
        if (recorder.state === "paused") {
          const previewBlob = new Blob(voiceChunksRef.current, { type: recorder.mimeType || "audio/webm" });
          if (previewBlob.size > 0) {
            setPreviewBlob(previewBlob);
          }
        }
      };

      recorder.onerror = () => {
        detachRecorder(recorder);
        stopVoiceStreamTracks();
        voiceChunksRef.current = [];
        resetVoiceTicker();
        clearPreview();
        setVoiceBlob(null);
        setVoiceRecorderState("idle");
        setVoiceError(
          langRef.current === "es"
            ? "No se pudo completar la grabacion."
            : "Unable to complete recording.",
        );
      };

      recorder.onstop = () => {
        detachRecorder(recorder);
        stopVoiceStreamTracks();
        stopVoiceTicker();

        const chunks = voiceChunksRef.current;
        voiceChunksRef.current = [];
        if (chunks.length === 0) {
          clearPreview();
          setVoiceRecorderState("idle");
          setVoiceBlob(null);
          setVoiceError(
            langRef.current === "es"
              ? "No se detecto audio. Intente grabar de nuevo."
              : "No audio detected. Please record again.",
          );
          return;
        }

        const blob = new Blob(chunks, { type: recorder.mimeType || "audio/webm" });
        if (blob.size === 0) {
          clearPreview();
          setVoiceRecorderState("idle");
          setVoiceBlob(null);
          setVoiceError(
            langRef.current === "es"
              ? "No se detecto audio. Intente grabar de nuevo."
              : "No audio detected. Please record again.",
          );
          return;
        }

        setVoiceBlob(blob);
        setPreviewBlob(blob);
        setVoiceRecorderState("stopped");
      };

      mediaRecorderRef.current = recorder;
      voiceChunksRef.current = [];
      resetVoiceTicker();
      clearPreview();
      setVoiceBlob(null);
      setVoiceError("");
      setVoiceInfo("");
      setVoiceRecorderState("recording");
      recorder.start(250);
      startVoiceTicker();
    } catch {
      detachRecorder();
      stopVoiceStreamTracks();
      voiceChunksRef.current = [];
      resetVoiceTicker();
      clearPreview();
      setVoiceBlob(null);
      setVoiceRecorderState("idle");
      setVoiceError(
        langRef.current === "es"
          ? "No se pudo acceder al microfono."
          : "Unable to access microphone.",
      );
    }
  }, [
    clearPreview,
    detachRecorder,
    discardVoiceRecording,
    isActive,
    pauseVoiceRecording,
    resetVoiceTicker,
    resumeVoiceRecording,
    setPreviewBlob,
    startVoiceTicker,
    stopVoiceStreamTracks,
    voiceRecorderState,
  ]);

  const toggleVoicePreviewPlayback = useCallback(async () => {
    try {
      await togglePlayback();
    } catch {
      setVoiceError(
        langRef.current === "es"
          ? "No se pudo reproducir el audio grabado."
          : "Unable to play the recorded audio.",
      );
    }
  }, [togglePlayback]);

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
      language: langRef.current,
    });

    stopPlayback();
    setVoiceRecorderState("sending");
    setVoiceError("");
    setVoiceInfo(
      langRef.current === "es"
        ? "Enviando audio para transcripcion..."
        : "Uploading audio for transcription...",
    );

    try {
      const transcript = await transcribeAudio(audioBlob, langRef.current, trimmedSessionCode);
      setVoiceRecorderState("stopped");
      setVoiceInfo(
        langRef.current === "es"
          ? "Transcripcion lista. Enviando respuesta..."
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
          : (langRef.current === "es" ? "Error de transcripcion." : "Transcription error."),
      );
      return null;
    }
  }, [isActive, stopPlayback, voiceBlob, voiceRecorderState]);

  useEffect(() => {
    if (isActive || voiceRecorderState === "idle") return;
    discardVoiceRecording();
  }, [discardVoiceRecording, isActive, voiceRecorderState]);

  useEffect(() => {
    return () => {
      const recorder = mediaRecorderRef.current;
      if (recorder) {
        detachRecorder(recorder);
        if (recorder.state !== "inactive") {
          try {
            recorder.stop();
          } catch {
            // Ignore recorder stop errors during unmount.
          }
        }
      }
      stopVoiceStreamTracks();
    };
  }, [detachRecorder, stopVoiceStreamTracks]);

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
