"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { Dispatch, MutableRefObject, SetStateAction } from "react";
import type { MicWarmState, VoiceFeedback } from "../viewTypes";

export type StreamAcquireMode = "prepare" | "recover" | "start";

export interface ReleaseWarmStreamOptions {
  stopTracks?: boolean;
  nextState?: MicWarmState;
}

interface UseWarmStreamParams {
  generationRef: MutableRefObject<number>;
  shouldKeepMicWarm: boolean;
  isRecorderActive: () => boolean;
  setVoiceError: Dispatch<SetStateAction<VoiceFeedback | null>>;
}

interface UseWarmStreamResult {
  micWarmState: MicWarmState;
  mediaStream: MediaStream | null;
  prepareMicrophone: () => Promise<boolean>;
  ensureWarmStream: (mode: StreamAcquireMode) => Promise<MediaStream | null>;
  releaseWarmStream: (options?: ReleaseWarmStreamOptions) => void;
  stopEphemeralStreamIfNeeded: () => void;
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

export function useWarmStream({
  generationRef,
  shouldKeepMicWarm,
  isRecorderActive,
  setVoiceError,
}: UseWarmStreamParams): UseWarmStreamResult {
  const [micWarmState, setMicWarmState] = useState<MicWarmState>("cold");
  const mediaStreamRef = useRef<MediaStream | null>(null);
  const warmStreamPromiseRef = useRef<Promise<MediaStream | null> | null>(null);
  const clearWarmStreamListenersRef = useRef<(() => void) | null>(null);
  const shouldKeepMicWarmRef = useRef(shouldKeepMicWarm);
  shouldKeepMicWarmRef.current = shouldKeepMicWarm;

  const isWarmStreamLive = useCallback((stream: MediaStream | null): boolean => {
    const tracks = getStreamTracks(stream);
    return tracks.length > 0 && tracks.every((track) => track.readyState !== "ended");
  }, []);

  const detachWarmStreamListeners = useCallback(() => {
    clearWarmStreamListenersRef.current?.();
    clearWarmStreamListenersRef.current = null;
  }, []);

  const releaseWarmStream = useCallback((options?: ReleaseWarmStreamOptions) => {
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
        setVoiceError({ code: "browser_unsupported" });
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
        setVoiceError(null);
        return stream;
      })
      .catch((error: unknown) => {
        if (generation !== generationRef.current) return null;

        const nextState = isPermissionDeniedError(error) ? "denied" : "error";
        setMicWarmState(nextState);

        if (mode === "start") {
          setVoiceError({
            code: nextState === "denied"
              ? "microphone_permission_denied"
              : "microphone_unavailable",
          });
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
  }, [cacheWarmStream, generationRef, isWarmStreamLive, releaseWarmStream, setVoiceError]);

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
    generationRef.current += 1;
    releaseWarmStream({ stopTracks: true, nextState: "cold" });
  }, [generationRef, releaseWarmStream, shouldKeepMicWarm]);

  useEffect(() => {
    if (!shouldKeepMicWarm || micWarmState !== "recovering") return;
    if (isRecorderActive()) return;
    void ensureWarmStream("recover");
  }, [ensureWarmStream, isRecorderActive, micWarmState, shouldKeepMicWarm]);

  useEffect(() => {
    if (!shouldKeepMicWarm) return;

    const maybeRecoverWarmStream = () => {
      if (isRecorderActive()) return;
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
  }, [ensureWarmStream, isRecorderActive, isWarmStreamLive, shouldKeepMicWarm]);

  return {
    micWarmState,
    mediaStream: mediaStreamRef.current,
    prepareMicrophone,
    ensureWarmStream,
    releaseWarmStream,
    stopEphemeralStreamIfNeeded,
  };
}
