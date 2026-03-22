"use client";

import { useCallback, useRef, useState } from "react";
import type { Dispatch, MutableRefObject, SetStateAction } from "react";
import {
  VOICE_CHUNK_TIMESLICE_MS,
  VOICE_MIME_CANDIDATES,
} from "../constants";
import type { ReleaseWarmStreamOptions, StreamAcquireMode } from "./useWarmStream";
import type { VoiceFeedback, VoiceRecorderState } from "../viewTypes";

interface UseMediaRecorderCoreParams {
  generationRef: MutableRefObject<number>;
  recorderActiveRef: MutableRefObject<boolean>;
  isActive: boolean;
  shouldKeepMicWarm: boolean;
  ensureWarmStream: (mode: StreamAcquireMode) => Promise<MediaStream | null>;
  releaseWarmStream: (options?: ReleaseWarmStreamOptions) => void;
  stopEphemeralStreamIfNeeded: () => void;
  startVoiceTicker: () => void;
  pauseVoiceTicker: () => void;
  resumeVoiceTicker: () => void;
  stopVoiceTicker: () => void;
  resetVoiceTicker: () => void;
  setPreviewBlob: (blob: Blob | null) => void;
  clearPreview: () => void;
  stopPlayback: () => void;
  setVoiceError: Dispatch<SetStateAction<VoiceFeedback | null>>;
  setVoiceInfo: Dispatch<SetStateAction<VoiceFeedback | null>>;
  voiceError: VoiceFeedback | null;
  voiceInfo: VoiceFeedback | null;
}

interface UseMediaRecorderCoreResult {
  voiceRecorderState: VoiceRecorderState;
  setVoiceRecorderState: Dispatch<SetStateAction<VoiceRecorderState>>;
  voiceBlob: Blob | null;
  voiceBlobRef: MutableRefObject<Blob | null>;
  startVoiceRecording: () => Promise<void>;
  stopVoiceRecordingAndWaitForBlob: () => Promise<Blob | null>;
  completeVoiceRecording: () => void;
  pauseVoiceRecording: () => void;
  resumeVoiceRecording: () => void;
  discardVoiceRecording: () => void;
  cleanupRecorder: () => void;
  resetAfterReviewFailure: () => void;
}

export function useMediaRecorderCore({
  generationRef,
  recorderActiveRef,
  isActive,
  shouldKeepMicWarm,
  ensureWarmStream,
  releaseWarmStream,
  stopEphemeralStreamIfNeeded,
  startVoiceTicker,
  pauseVoiceTicker,
  resumeVoiceTicker,
  stopVoiceTicker,
  resetVoiceTicker,
  setPreviewBlob,
  clearPreview,
  stopPlayback,
  setVoiceError,
  setVoiceInfo,
  voiceError,
  voiceInfo,
}: UseMediaRecorderCoreParams): UseMediaRecorderCoreResult {
  const [voiceRecorderState, setVoiceRecorderState] = useState<VoiceRecorderState>("idle");
  const [voiceBlob, setVoiceBlob] = useState<Blob | null>(null);
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const voiceChunksRef = useRef<BlobPart[]>([]);
  const stopRecordingResolverRef = useRef<((blob: Blob | null) => void) | null>(null);
  const voiceBlobRef = useRef<Blob | null>(voiceBlob);
  voiceBlobRef.current = voiceBlob;

  const invalidateDeferredWork = useCallback(() => {
    generationRef.current += 1;
  }, [generationRef]);

  const scheduleGuardedTask = useCallback((generation: number, task: () => void) => {
    window.setTimeout(() => {
      if (generation !== generationRef.current) return;
      task();
    }, 0);
  }, [generationRef]);

  const detachRecorder = useCallback((recorder: MediaRecorder | null = mediaRecorderRef.current) => {
    if (!recorder) return;
    recorder.ondataavailable = null;
    recorder.onstop = null;
    recorder.onerror = null;
    if (mediaRecorderRef.current === recorder) {
      mediaRecorderRef.current = null;
    }
    recorderActiveRef.current = false;
  }, [recorderActiveRef]);

  const resolvePendingStop = useCallback((blob: Blob | null) => {
    if (!stopRecordingResolverRef.current) return;
    stopRecordingResolverRef.current(blob);
    stopRecordingResolverRef.current = null;
  }, []);

  const cleanupRecorder = useCallback(() => {
    resolvePendingStop(null);
    const recorder = mediaRecorderRef.current;
    if (!recorder) {
      recorderActiveRef.current = false;
      return;
    }
    detachRecorder(recorder);
    if (recorder.state !== "inactive") {
      try {
        recorder.stop();
      } catch {
        // Ignore recorder stop errors during cleanup.
      }
    }
  }, [detachRecorder, recorderActiveRef, resolvePendingStop]);

  const discardVoiceRecording = useCallback(() => {
    invalidateDeferredWork();
    cleanupRecorder();
    voiceChunksRef.current = [];
    resetVoiceTicker();
    clearPreview();
    setVoiceBlob(null);
    setVoiceRecorderState("idle");
    setVoiceError(null);
    setVoiceInfo(null);
    stopEphemeralStreamIfNeeded();
  }, [
    cleanupRecorder,
    clearPreview,
    invalidateDeferredWork,
    resetVoiceTicker,
    setVoiceError,
    setVoiceInfo,
    stopEphemeralStreamIfNeeded,
  ]);

  const resetAfterReviewFailure = useCallback(() => {
    invalidateDeferredWork();
    cleanupRecorder();
    voiceChunksRef.current = [];
    resetVoiceTicker();
    clearPreview();
    setVoiceBlob(null);
    setVoiceRecorderState("idle");
    setVoiceInfo(null);
    stopEphemeralStreamIfNeeded();
  }, [
    cleanupRecorder,
    clearPreview,
    invalidateDeferredWork,
    resetVoiceTicker,
    setVoiceInfo,
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
    recorderActiveRef.current = true;
    setVoiceRecorderState("paused");
  }, [pauseVoiceTicker, recorderActiveRef]);

  const resumeVoiceRecording = useCallback(() => {
    const recorder = mediaRecorderRef.current;
    if (!recorder || recorder.state !== "paused") return;
    stopPlayback();
    recorder.resume();
    recorderActiveRef.current = true;
    setVoiceRecorderState("recording");
    resumeVoiceTicker();
  }, [recorderActiveRef, resumeVoiceTicker, stopPlayback]);

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
      setVoiceError({ code: "secure_context_required" });
      return;
    }

    if (typeof MediaRecorder === "undefined") {
      setVoiceError({ code: "browser_unsupported" });
      return;
    }

    const isPristineIdle =
      voiceRecorderState === "idle"
      && voiceError == null
      && voiceInfo == null
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
        setVoiceInfo(null);
        setVoiceError({ code: "recording_failed" });
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
            setVoiceError({ code: "no_audio_detected" });
            stopEphemeralStreamIfNeeded();
            return;
          }

          const blob = new Blob(chunks, { type: recorder.mimeType || "audio/webm" });
          if (blob.size === 0) {
            clearPreview();
            setVoiceRecorderState("idle");
            setVoiceBlob(null);
            resolvePendingStop(null);
            setVoiceError({ code: "no_audio_detected" });
            stopEphemeralStreamIfNeeded();
            return;
          }

          setVoiceBlob(blob);
          setPreviewBlob(blob);
          setVoiceRecorderState("audio_ready");
          setVoiceInfo({ code: "audio_ready" });
          resolvePendingStop(blob);
          stopEphemeralStreamIfNeeded();
        });
      };

      mediaRecorderRef.current = recorder;
      recorderActiveRef.current = true;
      voiceChunksRef.current = [];
      resetVoiceTicker();
      clearPreview();
      setVoiceBlob(null);
      setVoiceError(null);
      setVoiceInfo(null);
      setVoiceRecorderState("recording");
      recorder.start(VOICE_CHUNK_TIMESLICE_MS);
      startVoiceTicker();
    } catch {
      detachRecorder();
      releaseWarmStream({
        stopTracks: true,
        nextState: shouldKeepMicWarm ? "error" : "cold",
      });
      invalidateDeferredWork();
      voiceChunksRef.current = [];
      resolvePendingStop(null);
      resetVoiceTicker();
      clearPreview();
      setVoiceBlob(null);
      setVoiceRecorderState("idle");
      setVoiceInfo(null);
      setVoiceError({ code: "microphone_unavailable" });
    }
  }, [
    clearPreview,
    detachRecorder,
    discardVoiceRecording,
    ensureWarmStream,
    generationRef,
    invalidateDeferredWork,
    isActive,
    pauseVoiceRecording,
    recorderActiveRef,
    releaseWarmStream,
    resetVoiceTicker,
    resolvePendingStop,
    resumeVoiceRecording,
    scheduleGuardedTask,
    setPreviewBlob,
    setVoiceError,
    setVoiceInfo,
    shouldKeepMicWarm,
    startVoiceTicker,
    stopEphemeralStreamIfNeeded,
    stopVoiceTicker,
    voiceError,
    voiceInfo,
    voiceRecorderState,
  ]);

  return {
    voiceRecorderState,
    setVoiceRecorderState,
    voiceBlob,
    voiceBlobRef,
    startVoiceRecording,
    stopVoiceRecordingAndWaitForBlob,
    completeVoiceRecording,
    pauseVoiceRecording,
    resumeVoiceRecording,
    discardVoiceRecording,
    cleanupRecorder,
    resetAfterReviewFailure,
  };
}
