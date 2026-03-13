"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { Lang } from "@/lib/language";
import {
  VOICE_MAX_SECONDS,
  VOICE_TICK_INTERVAL_MS,
  VOICE_WARNING_SECONDS,
} from "../constants";
import { createReviewVoiceRecording } from "../lib/createReviewVoiceRecording";
import type { MicWarmState, VoiceFeedback, VoiceRecorderState } from "../viewTypes";
import { useMediaRecorderCore } from "./useMediaRecorderCore";
import { useVoicePreview } from "./useVoicePreview";
import { useVoiceTicker } from "./useVoiceTicker";
import { useWarmStream } from "./useWarmStream";

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
  voiceError: VoiceFeedback | null;
  voiceInfo: VoiceFeedback | null;
  isRecordingActive: boolean;
  isRecordingPaused: boolean;
  prepareMicrophone: () => Promise<boolean>;
  startVoiceRecording: () => Promise<void>;
  completeVoiceRecording: () => void;
  discardVoiceRecording: () => void;
  toggleVoicePreviewPlayback: () => Promise<void>;
  reviewVoiceRecording: (sessionCode: string) => Promise<string | null>;
  setVoiceErrorFeedback: (feedback: VoiceFeedback | null) => void;
}

export function useVoiceRecorder({
  lang,
  isActive,
  shouldKeepMicWarm,
}: UseVoiceRecorderParams): UseVoiceRecorderResult {
  const [voiceError, setVoiceError] = useState<VoiceFeedback | null>(null);
  const [voiceInfo, setVoiceInfo] = useState<VoiceFeedback | null>(null);

  const generationRef = useRef(0);
  const recorderActiveRef = useRef(false);
  const langRef = useRef(lang);
  const completeVoiceRecordingRef = useRef<() => void>(() => {});
  langRef.current = lang;

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
      setVoiceInfo({ code: "limit_reached" });
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

  const isRecorderActive = useCallback(() => recorderActiveRef.current, []);

  const {
    micWarmState,
    prepareMicrophone,
    ensureWarmStream,
    releaseWarmStream,
    stopEphemeralStreamIfNeeded,
  } = useWarmStream({
    generationRef,
    shouldKeepMicWarm,
    isRecorderActive,
    setVoiceError,
  });

  const {
    voiceRecorderState,
    setVoiceRecorderState,
    voiceBlob,
    voiceBlobRef,
    startVoiceRecording,
    completeVoiceRecording,
    discardVoiceRecording,
    cleanupRecorder,
    resetAfterReviewFailure,
  } = useMediaRecorderCore({
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
  });
  completeVoiceRecordingRef.current = completeVoiceRecording;

  const toggleVoicePreviewPlayback = useCallback(async () => {
    try {
      await togglePlayback();
    } catch {
      setVoiceError({ code: "audio_playback_failed" });
    }
  }, [togglePlayback]);

  const reviewVoiceRecording = useCallback(createReviewVoiceRecording({
    isActive,
    voiceRecorderState,
    voiceBlobRef,
    langRef,
    stopPlayback,
    setVoiceRecorderState,
    setVoiceError,
    setVoiceInfo,
    resetAfterReviewFailure,
  }), [
    isActive,
    resetAfterReviewFailure,
    setVoiceRecorderState,
    stopPlayback,
    voiceBlobRef,
    voiceRecorderState,
  ]);

  useEffect(() => {
    if (isActive || voiceRecorderState === "idle") return;
    discardVoiceRecording();
  }, [discardVoiceRecording, isActive, voiceRecorderState]);

  useEffect(() => {
    return () => {
      generationRef.current += 1;
      cleanupRecorder();
      releaseWarmStream({ stopTracks: true, nextState: "cold" });
    };
  }, [cleanupRecorder, releaseWarmStream]);

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
    setVoiceErrorFeedback: setVoiceError,
  };
}
