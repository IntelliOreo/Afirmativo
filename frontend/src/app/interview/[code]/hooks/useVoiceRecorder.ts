"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { log } from "@/lib/logger";
import type { Lang } from "@/lib/language";
import {
  formatDurationLabel,
  VOICE_CHUNK_TIMESLICE_MS,
  VOICE_MAX_SECONDS,
  VOICE_MIME_CANDIDATES,
  VOICE_TICK_INTERVAL_MS,
  VOICE_WARNING_SECONDS,
} from "../constants";
import { transcribeAudio } from "../lib/voiceTranscription";
import type { MicWarmState, VoiceRecorderState } from "../viewTypes";
import { useVoicePreview } from "./useVoicePreview";
import { useVoiceTicker } from "./useVoiceTicker";

type StreamAcquireMode = "prepare" | "recover" | "start";

interface UseVoiceRecorderParams {
  lang: Lang;
  isActive: boolean;
  shouldKeepMicWarm: boolean;
}

interface UseVoiceRecorderResult {
  voiceRecorderState: VoiceRecorderState;
  micWarmState: MicWarmState;
  voiceDurationSeconds: number;
  voiceWarningSeconds: number | null;
  voiceBlob: Blob | null;
  voicePreviewUrl: string | null;
  isVoicePreviewPlaying: boolean;
  voiceError: string;
  voiceInfo: string;
  isRecordingActive: boolean;
  isRecordingPaused: boolean;
  prepareMicrophone: () => Promise<boolean>;
  startVoiceRecording: () => Promise<void>;
  completeVoiceRecording: () => void;
  discardVoiceRecording: () => void;
  toggleVoicePreviewPlayback: () => Promise<void>;
  reviewVoiceRecording: (sessionCode: string) => Promise<string | null>;
  setVoiceErrorMessage: (message: string) => void;
}

function getStreamTracks(stream: MediaStream | null): MediaStreamTrack[] {
  if (!stream) return [];
  if (typeof stream.getAudioTracks === "function") {
    const audioTracks = stream.getAudioTracks();
    if (audioTracks.length > 0) {
      return audioTracks;
    }
  }
  return typeof stream.getTracks === "function" ? stream.getTracks() : [];
}

function isPermissionDeniedError(error: unknown): boolean {
  if (!error || typeof error !== "object") return false;
  const maybeName = "name" in error ? String(error.name) : "";
  return maybeName === "NotAllowedError" || maybeName === "PermissionDeniedError";
}

export function useVoiceRecorder({
  lang,
  isActive,
  shouldKeepMicWarm,
}: UseVoiceRecorderParams): UseVoiceRecorderResult {
  const [voiceRecorderState, setVoiceRecorderState] = useState<VoiceRecorderState>("idle");
  const [micWarmState, setMicWarmState] = useState<MicWarmState>("cold");
  const [voiceBlob, setVoiceBlob] = useState<Blob | null>(null);
  const [voiceError, setVoiceError] = useState("");
  const [voiceInfo, setVoiceInfo] = useState("");

  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const mediaStreamRef = useRef<MediaStream | null>(null);
  const voiceChunksRef = useRef<BlobPart[]>([]);
  const stopRecordingResolverRef = useRef<((blob: Blob | null) => void) | null>(null);
  const warmStreamPromiseRef = useRef<Promise<MediaStream | null> | null>(null);
  const clearWarmStreamListenersRef = useRef<(() => void) | null>(null);
  const generationRef = useRef(0);
  const langRef = useRef(lang);
  const shouldKeepMicWarmRef = useRef(shouldKeepMicWarm);
  const voiceBlobRef = useRef<Blob | null>(voiceBlob);
  langRef.current = lang;
  shouldKeepMicWarmRef.current = shouldKeepMicWarm;
  voiceBlobRef.current = voiceBlob;

  const microphoneAccessMessage = useCallback((permissionDenied: boolean) => (
    permissionDenied
      ? (langRef.current === "es"
        ? "Permita el acceso al micrófono para grabar por voz."
        : "Allow microphone access to record by voice.")
      : (langRef.current === "es"
        ? "No se pudo acceder al micrófono."
        : "Unable to access microphone.")
  ), []);

  const recordingFailureMessage = useCallback(() => (
    langRef.current === "es"
      ? "No se pudo completar la grabación."
      : "Unable to complete recording."
  ), []);

  const noAudioDetectedMessage = useCallback(() => (
    langRef.current === "es"
      ? "No se detecto audio. Intente grabar de nuevo."
      : "No audio detected. Please record again."
  ), []);

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
    tickIntervalMs: VOICE_TICK_INTERVAL_MS,
    onLimitReached: () => {
      const limitLabel = formatDurationLabel(VOICE_MAX_SECONDS, langRef.current);
      setVoiceInfo(
        langRef.current === "es"
          ? `Se alcanzó el límite de ${limitLabel} y la grabación se detuvo.`
          : `The limit of ${limitLabel} was reached and recording stopped.`,
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

  const invalidateDeferredWork = useCallback(() => {
    generationRef.current += 1;
  }, []);

  const scheduleGuardedTask = useCallback((generation: number, task: () => void) => {
    window.setTimeout(() => {
      if (generation !== generationRef.current) return;
      task();
    }, 0);
  }, []);

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

  const isWarmStreamLive = useCallback((stream: MediaStream | null): boolean => {
    const tracks = getStreamTracks(stream);
    return tracks.length > 0 && tracks.every((track) => track.readyState !== "ended");
  }, []);

  const detachWarmStreamListeners = useCallback(() => {
    clearWarmStreamListenersRef.current?.();
    clearWarmStreamListenersRef.current = null;
  }, []);

  const releaseWarmStream = useCallback((options?: { stopTracks?: boolean; nextState?: MicWarmState }) => {
    const stream = mediaStreamRef.current;
    detachWarmStreamListeners();
    mediaStreamRef.current = null;
    warmStreamPromiseRef.current = null;

    if (stream && options?.stopTracks) {
      stream.getTracks().forEach((track) => {
        try {
          track.stop();
        } catch {
          // Ignore track stop failures during cleanup.
        }
      });
    }

    if (options?.nextState) {
      setMicWarmState(options.nextState);
    }
  }, [detachWarmStreamListeners]);

  const cacheWarmStream = useCallback((stream: MediaStream) => {
    detachWarmStreamListeners();
    mediaStreamRef.current = stream;

    const tracks = getStreamTracks(stream);
    const handleTrackEnded = () => {
      if (mediaStreamRef.current !== stream) return;
      releaseWarmStream({
        nextState: shouldKeepMicWarmRef.current ? "recovering" : "cold",
      });
    };

    tracks.forEach((track) => {
      track.addEventListener("ended", handleTrackEnded);
    });
    clearWarmStreamListenersRef.current = () => {
      tracks.forEach((track) => {
        track.removeEventListener("ended", handleTrackEnded);
      });
    };

    setMicWarmState("warm");
  }, [detachWarmStreamListeners, releaseWarmStream]);

  const ensureWarmStream = useCallback(async (mode: StreamAcquireMode): Promise<MediaStream | null> => {
    const existingStream = mediaStreamRef.current;
    if (isWarmStreamLive(existingStream)) {
      setMicWarmState("warm");
      return existingStream;
    }

    if (existingStream) {
      releaseWarmStream({
        nextState: mode === "recover" ? "recovering" : "cold",
      });
    }

    if (mode === "recover" && !shouldKeepMicWarmRef.current) {
      return null;
    }

    if (warmStreamPromiseRef.current) {
      return warmStreamPromiseRef.current;
    }

    if (
      typeof navigator === "undefined"
      || !navigator.mediaDevices?.getUserMedia
    ) {
      if (mode === "start") {
        setVoiceError(
          langRef.current === "es"
            ? "Este navegador no soporta grabacion de audio."
            : "This browser does not support audio recording.",
        );
      } else {
        setMicWarmState("error");
      }
      return null;
    }

    const generation = generationRef.current;
    setMicWarmState(mode === "recover" ? "recovering" : "warming");

    const acquisitionPromise: Promise<MediaStream | null> = navigator.mediaDevices.getUserMedia({ audio: true })
      .then((stream) => {
        if (
          generation !== generationRef.current
          || (mode === "recover" && !shouldKeepMicWarmRef.current)
        ) {
          stream.getTracks().forEach((track) => track.stop());
          return null;
        }

        cacheWarmStream(stream);
        setVoiceError("");
        return stream;
      })
      .catch((error: unknown) => {
        if (generation !== generationRef.current) return null;

        const nextState = isPermissionDeniedError(error) ? "denied" : "error";
        setMicWarmState(nextState);

        if (mode === "start") {
          setVoiceError(microphoneAccessMessage(nextState === "denied"));
        }
        return null;
      })
      .finally(() => {
        if (warmStreamPromiseRef.current === acquisitionPromise) {
          warmStreamPromiseRef.current = null;
        }
      });

    warmStreamPromiseRef.current = acquisitionPromise;
    return acquisitionPromise;
  }, [cacheWarmStream, isWarmStreamLive, microphoneAccessMessage, releaseWarmStream]);

  const prepareMicrophone = useCallback(async (): Promise<boolean> => {
    const stream = await ensureWarmStream("prepare");
    return stream != null;
  }, [ensureWarmStream]);

  const stopEphemeralStreamIfNeeded = useCallback(() => {
    if (shouldKeepMicWarmRef.current) return;
    releaseWarmStream({ stopTracks: true, nextState: "cold" });
  }, [releaseWarmStream]);

  useEffect(() => {
    if (shouldKeepMicWarm) return;
    invalidateDeferredWork();
    releaseWarmStream({ stopTracks: true, nextState: "cold" });
  }, [invalidateDeferredWork, releaseWarmStream, shouldKeepMicWarm]);

  useEffect(() => {
    if (!shouldKeepMicWarm || micWarmState !== "recovering") return;
    const recorder = mediaRecorderRef.current;
    if (recorder && recorder.state !== "inactive") return;
    void ensureWarmStream("recover");
  }, [ensureWarmStream, micWarmState, shouldKeepMicWarm]);

  useEffect(() => {
    if (!shouldKeepMicWarm) return;

    const maybeRecoverWarmStream = () => {
      const recorder = mediaRecorderRef.current;
      if (recorder && recorder.state !== "inactive") return;
      if (isWarmStreamLive(mediaStreamRef.current)) {
        setMicWarmState("warm");
        return;
      }
      setMicWarmState("recovering");
      void ensureWarmStream("recover");
    };

    const handleVisibilityChange = () => {
      if (document.hidden) return;
      maybeRecoverWarmStream();
    };

    window.addEventListener("focus", maybeRecoverWarmStream);
    document.addEventListener("visibilitychange", handleVisibilityChange);

    return () => {
      window.removeEventListener("focus", maybeRecoverWarmStream);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, [ensureWarmStream, isWarmStreamLive, shouldKeepMicWarm]);

  const discardVoiceRecording = useCallback(() => {
    invalidateDeferredWork();
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

    voiceChunksRef.current = [];
    resolvePendingStop(null);
    resetVoiceTicker();
    clearPreview();
    setVoiceBlob(null);
    setVoiceRecorderState("idle");
    setVoiceError("");
    setVoiceInfo("");
    stopEphemeralStreamIfNeeded();
  }, [
    clearPreview,
    detachRecorder,
    invalidateDeferredWork,
    resetVoiceTicker,
    resolvePendingStop,
    stopEphemeralStreamIfNeeded,
  ]);

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

    if (typeof MediaRecorder === "undefined") {
      setVoiceError(
        langRef.current === "es"
          ? "Este navegador no soporta grabacion de audio."
          : "This browser does not support audio recording.",
      );
      return;
    }

    const isPristineIdle =
      voiceRecorderState === "idle"
      && voiceError === ""
      && voiceInfo === ""
      && voiceBlobRef.current == null;
    if (!isPristineIdle) {
      discardVoiceRecording();
    }

    try {
      const stream = await ensureWarmStream("start");
      if (!stream) return;

      const mimeType = VOICE_MIME_CANDIDATES.find((candidate) => MediaRecorder.isTypeSupported(candidate));
      const recorder = mimeType
        ? new MediaRecorder(stream, { mimeType })
        : new MediaRecorder(stream);

      recorder.ondataavailable = (event: BlobEvent) => {
        if (event.data.size <= 0) return;

        voiceChunksRef.current.push(event.data);
        if (recorder.state === "paused") {
          const generation = generationRef.current;
          scheduleGuardedTask(generation, () => {
            const previewBlob = new Blob(voiceChunksRef.current, { type: recorder.mimeType || "audio/webm" });
            if (previewBlob.size > 0) {
              setPreviewBlob(previewBlob);
            }
          });
        }
      };

      recorder.onerror = () => {
        invalidateDeferredWork();
        detachRecorder(recorder);
        releaseWarmStream({ stopTracks: true, nextState: "error" });
        voiceChunksRef.current = [];
        resolvePendingStop(null);
        resetVoiceTicker();
        clearPreview();
        setVoiceBlob(null);
        setVoiceRecorderState("idle");
        setVoiceInfo("");
        setVoiceError(recordingFailureMessage());
      };

      recorder.onstop = () => {
        detachRecorder(recorder);
        stopVoiceTicker();
        const generation = generationRef.current;
        scheduleGuardedTask(generation, () => {
          const chunks = voiceChunksRef.current;
          voiceChunksRef.current = [];
          if (chunks.length === 0) {
            clearPreview();
            setVoiceRecorderState("idle");
            setVoiceBlob(null);
            resolvePendingStop(null);
            setVoiceError(noAudioDetectedMessage());
            stopEphemeralStreamIfNeeded();
            return;
          }

          const blob = new Blob(chunks, { type: recorder.mimeType || "audio/webm" });
          if (blob.size === 0) {
            clearPreview();
            setVoiceRecorderState("idle");
            setVoiceBlob(null);
            resolvePendingStop(null);
            setVoiceError(noAudioDetectedMessage());
            stopEphemeralStreamIfNeeded();
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
          stopEphemeralStreamIfNeeded();
        });
      };

      mediaRecorderRef.current = recorder;
      voiceChunksRef.current = [];
      resetVoiceTicker();
      clearPreview();
      setVoiceBlob(null);
      setVoiceError("");
      setVoiceInfo("");
      setVoiceRecorderState("recording");
      recorder.start(VOICE_CHUNK_TIMESLICE_MS);
      startVoiceTicker();
    } catch {
      detachRecorder();
      releaseWarmStream({
        stopTracks: true,
        nextState: shouldKeepMicWarmRef.current ? "error" : "cold",
      });
      invalidateDeferredWork();
      voiceChunksRef.current = [];
      resolvePendingStop(null);
      resetVoiceTicker();
      clearPreview();
      setVoiceBlob(null);
      setVoiceRecorderState("idle");
      setVoiceInfo("");
      setVoiceError(microphoneAccessMessage(false));
    }
  }, [
    clearPreview,
    detachRecorder,
    discardVoiceRecording,
    ensureWarmStream,
    invalidateDeferredWork,
    isActive,
    microphoneAccessMessage,
    noAudioDetectedMessage,
    pauseVoiceRecording,
    recordingFailureMessage,
    releaseWarmStream,
    resetVoiceTicker,
    resolvePendingStop,
    resumeVoiceRecording,
    scheduleGuardedTask,
    setPreviewBlob,
    startVoiceTicker,
    stopEphemeralStreamIfNeeded,
    stopVoiceTicker,
    voiceError,
    voiceInfo,
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

  useEffect(() => {
    if (isActive || voiceRecorderState === "idle") return;
    discardVoiceRecording();
  }, [discardVoiceRecording, isActive, voiceRecorderState]);

  useEffect(() => {
    return () => {
      invalidateDeferredWork();
      resolvePendingStop(null);
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
      releaseWarmStream({ stopTracks: true, nextState: "cold" });
    };
  }, [detachRecorder, invalidateDeferredWork, releaseWarmStream, resolvePendingStop]);

  return {
    voiceRecorderState,
    micWarmState,
    voiceDurationSeconds,
    voiceWarningSeconds,
    voiceBlob,
    voicePreviewUrl,
    isVoicePreviewPlaying,
    voiceError,
    voiceInfo,
    isRecordingActive: voiceRecorderState === "recording",
    isRecordingPaused: voiceRecorderState === "paused",
    prepareMicrophone,
    startVoiceRecording,
    completeVoiceRecording,
    discardVoiceRecording,
    toggleVoicePreviewPlayback,
    reviewVoiceRecording,
    setVoiceErrorMessage: setVoiceError,
  };
}
