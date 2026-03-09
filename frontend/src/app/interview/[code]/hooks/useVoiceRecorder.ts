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
  reviewVoiceRecording: (sessionCode: string) => Promise<string | null>;
  finalizeVoiceRecording: (sessionCode: string) => Promise<string | null>;
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
  const stopRecordingResolverRef = useRef<((blob: Blob | null) => void) | null>(null);
  const langRef = useRef(lang);
  langRef.current = lang;
  const voiceBlobRef = useRef<Blob | null>(voiceBlob);
  voiceBlobRef.current = voiceBlob;

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

  const resolvePendingStop = useCallback((blob: Blob | null) => {
    if (!stopRecordingResolverRef.current) return;
    stopRecordingResolverRef.current(blob);
    stopRecordingResolverRef.current = null;
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
    resolvePendingStop(null);
    resetVoiceTicker();
    clearPreview();
    setVoiceBlob(null);
    setVoiceRecorderState("idle");
    setVoiceError("");
    setVoiceInfo("");
  }, [clearPreview, detachRecorder, resetVoiceTicker, resolvePendingStop, stopVoiceStreamTracks]);

  const stopVoiceRecordingAndWaitForBlob = useCallback(async (): Promise<Blob | null> => {
    if (
      (voiceRecorderState === "audio_ready" || voiceRecorderState === "review_ready")
      && voiceBlobRef.current
    ) {
      return voiceBlobRef.current;
    }

    const recorder = mediaRecorderRef.current;
    if (!recorder || recorder.state === "inactive") {
      return voiceBlobRef.current;
    }

    return new Promise<Blob | null>((resolve) => {
      stopRecordingResolverRef.current = resolve;
      stopVoiceTicker();
      try {
        recorder.stop();
      } catch {
        resolvePendingStop(null);
      }
    });
  }, [resolvePendingStop, stopVoiceTicker, voiceRecorderState]);

  const completeVoiceRecording = useCallback(() => {
    void stopVoiceRecordingAndWaitForBlob();
  }, [stopVoiceRecordingAndWaitForBlob]);
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
          resolvePendingStop(null);
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
          resolvePendingStop(null);
          setVoiceError(
            langRef.current === "es"
              ? "No se detecto audio. Intente grabar de nuevo."
              : "No audio detected. Please record again.",
          );
          return;
        }

        setVoiceBlob(blob);
        setPreviewBlob(blob);
        setVoiceRecorderState("audio_ready");
        setVoiceInfo(
          langRef.current === "es"
            ? "Audio listo. Revise la transcripción antes de enviar."
            : "Audio ready. Review the transcript before submitting.",
        );
        resolvePendingStop(blob);
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

  const reviewVoiceRecording = useCallback(async (sessionCode: string): Promise<string | null> => {
    if (!isActive || voiceRecorderState !== "audio_ready" || !voiceBlobRef.current) return null;
    const trimmedSessionCode = sessionCode.trim();
    const audioBlob = voiceBlobRef.current;

    log.debug("voice review requested", {
      phase: "review_start",
      session_code: trimmedSessionCode,
      recorder_state: voiceRecorderState,
      blob_size_bytes: audioBlob.size,
      blob_type: audioBlob.type || "application/octet-stream",
      language: langRef.current,
    });

    stopPlayback();
    setVoiceRecorderState("transcribing_for_review");
    setVoiceError("");
    setVoiceInfo(
      langRef.current === "es"
        ? "Preparando la transcripción para revisar..."
        : "Preparing the transcript for review...",
    );

    try {
      const transcript = await transcribeAudio(audioBlob, langRef.current, trimmedSessionCode);
      setVoiceRecorderState("review_ready");
      setVoiceInfo(
        langRef.current === "es"
          ? "Transcripción lista. Revísela y envíe su respuesta."
          : "Transcript ready. Review it and submit your answer.",
      );
      log.info("voice review transcription completed", {
        phase: "review_done",
        transcript_length: transcript.length,
      });
      return transcript;
    } catch (err) {
      setVoiceRecorderState("audio_ready");
      setVoiceInfo(
        langRef.current === "es"
          ? "Audio listo. Puede intentar revisar la transcripción otra vez."
          : "Audio ready. You can try reviewing the transcript again.",
      );
      log.error("voice review transcription failed", {
        phase: "review_error",
        error: err instanceof Error ? err.message : "unknown_error",
      });
      setVoiceError(
        err instanceof Error
          ? err.message
          : (langRef.current === "es" ? "Error de transcripción." : "Transcription error."),
      );
      return null;
    }
  }, [isActive, stopPlayback, voiceRecorderState]);

  const finalizeVoiceRecording = useCallback(async (sessionCode: string): Promise<string | null> => {
    if (!isActive) return null;

    const trimmedSessionCode = sessionCode.trim();
    const audioBlob = await stopVoiceRecordingAndWaitForBlob();
    if (!audioBlob) return null;

    stopPlayback();
    setVoiceRecorderState("forced_finalizing");
    setVoiceError("");
    setVoiceInfo(
      langRef.current === "es"
        ? "Finalizando su respuesta..."
        : "Finalizing your answer...",
    );

    for (let attempt = 0; attempt < 2; attempt += 1) {
      try {
        const transcript = await transcribeAudio(audioBlob, langRef.current, trimmedSessionCode);
        log.info("voice forced-finalization transcription completed", {
          phase: "forced_finalization_done",
          transcript_length: transcript.length,
          attempt: attempt + 1,
        });
        return transcript;
      } catch (err) {
        log.error("voice forced-finalization transcription failed", {
          phase: "forced_finalization_error",
          attempt: attempt + 1,
          error: err instanceof Error ? err.message : "unknown_error",
        });
        if (attempt === 1) {
          setVoiceError(
            err instanceof Error
              ? err.message
              : (langRef.current === "es" ? "Error de transcripción." : "Transcription error."),
          );
        }
      }
    }

    return null;
  }, [isActive, stopPlayback, stopVoiceRecordingAndWaitForBlob]);

  useEffect(() => {
    if (isActive || voiceRecorderState === "idle" || voiceRecorderState === "forced_finalizing") return;
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
    reviewVoiceRecording,
    finalizeVoiceRecording,
    setVoiceErrorMessage: setVoiceError,
  };
}
