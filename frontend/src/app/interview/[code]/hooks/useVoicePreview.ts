"use client";

import { useCallback, useEffect, useRef, useState } from "react";

interface UseVoicePreviewResult {
  previewUrl: string | null;
  isPlaying: boolean;
  setPreviewBlob: (blob: Blob | null) => void;
  togglePlayback: () => Promise<void>;
  stopPlayback: () => void;
  clearPreview: () => void;
}

export function useVoicePreview(): UseVoicePreviewResult {
  const [previewBlob, setPreviewBlobState] = useState<Blob | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const previewAudioRef = useRef<HTMLAudioElement | null>(null);

  const releaseAudio = useCallback((audio: HTMLAudioElement | null = previewAudioRef.current) => {
    if (!audio) return;
    audio.pause();
    audio.currentTime = 0;
    audio.onplay = null;
    audio.onpause = null;
    audio.onended = null;
    if (previewAudioRef.current === audio) {
      previewAudioRef.current = null;
    }
  }, []);

  const stopPlayback = useCallback(() => {
    const audio = previewAudioRef.current;
    if (!audio) return;
    audio.pause();
    audio.currentTime = 0;
    setIsPlaying(false);
  }, []);

  const setPreviewBlob = useCallback((blob: Blob | null) => {
    setPreviewBlobState(blob);
  }, []);

  const clearPreview = useCallback(() => {
    stopPlayback();
    setPreviewBlobState(null);
    setPreviewUrl(null);
  }, [stopPlayback]);

  const togglePlayback = useCallback(async () => {
    const audio = previewAudioRef.current;
    if (!audio || !previewUrl) return;

    if (!audio.paused) {
      audio.pause();
      return;
    }

    if (audio.ended) {
      audio.currentTime = 0;
    }

    await audio.play();
  }, [previewUrl]);

  useEffect(() => {
    if (!previewBlob) {
      stopPlayback();
      releaseAudio();
      setPreviewUrl(null);
      return;
    }

    const url = URL.createObjectURL(previewBlob);
    setPreviewUrl(url);
    releaseAudio();
    setIsPlaying(false);

    const previewAudio = new Audio(url);
    previewAudio.onplay = () => setIsPlaying(true);
    previewAudio.onpause = () => setIsPlaying(false);
    previewAudio.onended = () => setIsPlaying(false);
    previewAudioRef.current = previewAudio;

    return () => {
      releaseAudio(previewAudio);
      URL.revokeObjectURL(url);
    };
  }, [previewBlob, releaseAudio, stopPlayback]);

  useEffect(() => {
    return () => {
      releaseAudio();
    };
  }, [releaseAudio]);

  return {
    previewUrl,
    isPlaying,
    setPreviewBlob,
    togglePlayback,
    stopPlayback,
    clearPreview,
  };
}
