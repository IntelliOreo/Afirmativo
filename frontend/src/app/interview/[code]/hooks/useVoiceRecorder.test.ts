import { renderHook } from "@testing-library/react";
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { VOICE_CHUNK_TIMESLICE_MS } from "../constants";
import { transcribeAudio } from "../lib/voiceTranscription";
import { useVoiceRecorder } from "./useVoiceRecorder";

const startTickerMock = vi.fn();
const resetVoiceTickerMock = vi.fn();
const stopVoiceTickerMock = vi.fn();
const pauseVoiceTickerMock = vi.fn();
const resumeVoiceTickerMock = vi.fn();
const setPreviewBlobMock = vi.fn();
const stopPlaybackMock = vi.fn();
const clearPreviewMock = vi.fn();
const getUserMediaMock = vi.fn();
const recorderStartMock = vi.fn();

interface MockTrack extends MediaStreamTrack {
  emitEnded: () => void;
}

interface MockStream extends MediaStream {
  getTracks: () => MockTrack[];
  getAudioTracks: () => MockTrack[];
}

function createDeferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function createTrack(): MockTrack {
  const listeners = new Map<string, Set<EventListener>>();
  const track = {
    kind: "audio",
    readyState: "live" as MediaStreamTrackState,
    stop: vi.fn(() => {
      track.readyState = "ended";
      track.emitEnded();
    }),
    addEventListener: vi.fn((type: string, listener: EventListener) => {
      const typedListeners = listeners.get(type) ?? new Set<EventListener>();
      typedListeners.add(listener);
      listeners.set(type, typedListeners);
    }),
    removeEventListener: vi.fn((type: string, listener: EventListener) => {
      listeners.get(type)?.delete(listener);
    }),
    dispatchEvent: vi.fn(() => true),
    emitEnded: () => {
      listeners.get("ended")?.forEach((listener) => listener(new Event("ended")));
    },
  } as unknown as MockTrack;

  return track;
}

function createStream(): { stream: MockStream; tracks: MockTrack[] } {
  const tracks = [createTrack()];
  return {
    stream: {
      getTracks: () => tracks,
      getAudioTracks: () => tracks,
    } as unknown as MockStream,
    tracks,
  };
}

vi.mock("./useVoiceTicker", () => ({
  useVoiceTicker: () => ({
    durationSeconds: 0,
    warningSeconds: null,
    start: startTickerMock,
    pause: pauseVoiceTickerMock,
    resume: resumeVoiceTickerMock,
    stop: stopVoiceTickerMock,
    reset: resetVoiceTickerMock,
  }),
}));

vi.mock("./useVoicePreview", () => ({
  useVoicePreview: () => ({
    previewUrl: null,
    isPlaying: false,
    setPreviewBlob: setPreviewBlobMock,
    togglePlayback: vi.fn(async () => {}),
    stopPlayback: stopPlaybackMock,
    clearPreview: clearPreviewMock,
  }),
}));

vi.mock("../lib/voiceTranscription", () => ({
  transcribeAudio: vi.fn(),
}));

class MockMediaRecorder {
  static isTypeSupported = vi.fn(() => true);
  static instances: MockMediaRecorder[] = [];

  state: RecordingState = "inactive";
  mimeType: string;
  ondataavailable: ((event: BlobEvent) => void) | null = null;
  onstop: (() => void) | null = null;
  onerror: (() => void) | null = null;

  constructor(_stream: MediaStream, options?: MediaRecorderOptions) {
    this.mimeType = options?.mimeType ?? "audio/webm";
    MockMediaRecorder.instances.push(this);
  }

  start(timeslice?: number): void {
    this.state = "recording";
    recorderStartMock(timeslice);
  }

  stop(): void {
    this.state = "inactive";
    this.onstop?.();
  }

  pause(): void {
    this.state = "paused";
  }

  resume(): void {
    this.state = "recording";
  }

  requestData(): void {
    window.setTimeout(() => {
      this.ondataavailable?.({ data: new Blob(["request-data"]) } as BlobEvent);
    }, 0);
  }

  emitData(text = "audio"): void {
    this.ondataavailable?.({ data: new Blob([text]) } as BlobEvent);
  }

  emitError(): void {
    this.onerror?.();
  }
}

async function flushMicrotasks(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

describe("useVoiceRecorder", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    startTickerMock.mockReset();
    resetVoiceTickerMock.mockReset();
    stopVoiceTickerMock.mockReset();
    pauseVoiceTickerMock.mockReset();
    resumeVoiceTickerMock.mockReset();
    setPreviewBlobMock.mockReset();
    stopPlaybackMock.mockReset();
    clearPreviewMock.mockReset();
    getUserMediaMock.mockReset();
    recorderStartMock.mockReset();
    vi.mocked(transcribeAudio).mockReset();
    MockMediaRecorder.instances = [];

    Object.defineProperty(window, "isSecureContext", {
      configurable: true,
      value: true,
    });
    Object.defineProperty(globalThis.navigator, "mediaDevices", {
      configurable: true,
      value: {
        getUserMedia: getUserMediaMock,
      },
    });
    Object.defineProperty(globalThis, "MediaRecorder", {
      configurable: true,
      value: MockMediaRecorder,
    });
    Object.defineProperty(document, "hidden", {
      configurable: true,
      value: false,
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("prepares the microphone once, reuses it for recording, and releases it when keep-warm ends", async () => {
    const first = createStream();
    getUserMediaMock.mockResolvedValue(first.stream);

    const { result, rerender } = renderHook(
      ({ shouldKeepMicWarm }) => useVoiceRecorder({
        lang: "en",
        isActive: true,
        shouldKeepMicWarm,
      }),
      { initialProps: { shouldKeepMicWarm: false } },
    );

    await act(async () => {
      expect(await result.current.prepareMicrophone()).toBe(true);
      await flushMicrotasks();
    });

    expect(result.current.micWarmState).toBe("warm");
    expect(getUserMediaMock).toHaveBeenCalledTimes(1);

    rerender({ shouldKeepMicWarm: true });

    await act(async () => {
      await result.current.startVoiceRecording();
    });

    expect(getUserMediaMock).toHaveBeenCalledTimes(1);
    expect(recorderStartMock).toHaveBeenCalledWith(VOICE_CHUNK_TIMESLICE_MS);

    rerender({ shouldKeepMicWarm: false });

    expect(first.tracks[0].stop).toHaveBeenCalledTimes(1);
    expect(result.current.micWarmState).toBe("cold");
  });

  it("stops a late warmup stream if the hook unmounts before preparation resolves", async () => {
    const first = createStream();
    const warmup = createDeferred<MockStream>();
    getUserMediaMock.mockReturnValueOnce(warmup.promise);

    const { result, unmount } = renderHook(() => useVoiceRecorder({
      lang: "en",
      isActive: true,
      shouldKeepMicWarm: false,
    }));

    act(() => {
      void result.current.prepareMicrophone();
    });

    unmount();

    await act(async () => {
      warmup.resolve(first.stream);
      await flushMicrotasks();
    });

    expect(first.tracks[0].stop).toHaveBeenCalledTimes(1);
  });

  it("surfaces denied microphone permission as a reconnectable state", async () => {
    getUserMediaMock.mockRejectedValueOnce({ name: "NotAllowedError" });

    const { result } = renderHook(() => useVoiceRecorder({
      lang: "en",
      isActive: true,
      shouldKeepMicWarm: false,
    }));

    await act(async () => {
      expect(await result.current.prepareMicrophone()).toBe(false);
      await flushMicrotasks();
    });

    expect(result.current.micWarmState).toBe("denied");
  });

  it("defers preview creation on pause until after the paused state is visible", async () => {
    const warmed = createStream();
    getUserMediaMock.mockResolvedValue(warmed.stream);

    const { result } = renderHook(() => useVoiceRecorder({
      lang: "en",
      isActive: true,
      shouldKeepMicWarm: true,
    }));

    await act(async () => {
      await result.current.prepareMicrophone();
      await flushMicrotasks();
      await result.current.startVoiceRecording();
    });

    await act(async () => {
      await result.current.startVoiceRecording();
    });

    expect(result.current.voiceRecorderState).toBe("paused");
    expect(setPreviewBlobMock).not.toHaveBeenCalled();

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(setPreviewBlobMock).toHaveBeenCalledTimes(1);
    expect(pauseVoiceTickerMock).toHaveBeenCalledTimes(1);
  });

  it("keeps the warm stream alive across stop and discard when keep-warm is enabled", async () => {
    const warmed = createStream();
    getUserMediaMock.mockResolvedValue(warmed.stream);

    const { result } = renderHook(() => useVoiceRecorder({
      lang: "en",
      isActive: true,
      shouldKeepMicWarm: true,
    }));

    await act(async () => {
      await result.current.prepareMicrophone();
      await flushMicrotasks();
      await result.current.startVoiceRecording();
    });

    MockMediaRecorder.instances[0]?.emitData("recorded");

    await act(async () => {
      result.current.completeVoiceRecording();
      await vi.runAllTimersAsync();
    });

    expect(result.current.voiceRecorderState).toBe("audio_ready");
    expect(warmed.tracks[0].stop).not.toHaveBeenCalled();

    act(() => {
      result.current.discardVoiceRecording();
    });

    expect(warmed.tracks[0].stop).not.toHaveBeenCalled();
  });

  it("releases an ephemeral stream after stop when keep-warm is disabled", async () => {
    const warmed = createStream();
    getUserMediaMock.mockResolvedValue(warmed.stream);

    const { result } = renderHook(() => useVoiceRecorder({
      lang: "en",
      isActive: true,
      shouldKeepMicWarm: false,
    }));

    await act(async () => {
      await result.current.startVoiceRecording();
    });

    MockMediaRecorder.instances[0]?.emitData("recorded");

    await act(async () => {
      result.current.completeVoiceRecording();
      await vi.runAllTimersAsync();
    });

    expect(warmed.tracks[0].stop).toHaveBeenCalledTimes(1);
    expect(result.current.voiceRecorderState).toBe("audio_ready");
  });

  it("drops deferred stop work after discard instead of restoring old audio", async () => {
    const warmed = createStream();
    getUserMediaMock.mockResolvedValue(warmed.stream);

    const { result } = renderHook(() => useVoiceRecorder({
      lang: "en",
      isActive: true,
      shouldKeepMicWarm: true,
    }));

    await act(async () => {
      await result.current.prepareMicrophone();
      await flushMicrotasks();
      await result.current.startVoiceRecording();
    });

    MockMediaRecorder.instances[0]?.emitData("recorded");

    await act(async () => {
      result.current.completeVoiceRecording();
    });

    act(() => {
      result.current.discardVoiceRecording();
    });

    await act(async () => {
      await vi.runAllTimersAsync();
    });

    expect(result.current.voiceRecorderState).toBe("idle");
    expect(result.current.voiceBlob).toBeNull();
  });

  it("stops an active recording and reviews it in one action", async () => {
    const warmed = createStream();
    getUserMediaMock.mockResolvedValue(warmed.stream);
    vi.mocked(transcribeAudio).mockResolvedValueOnce("transcribed text");

    const { result } = renderHook(() => useVoiceRecorder({
      lang: "en",
      isActive: true,
      shouldKeepMicWarm: true,
    }));

    await act(async () => {
      await result.current.prepareMicrophone();
      await flushMicrotasks();
      await result.current.startVoiceRecording();
    });

    MockMediaRecorder.instances[0]?.emitData("recorded");

    let transcript: string | null = null;
    await act(async () => {
      const reviewPromise = result.current.reviewVoiceRecording("AP-123");
      await vi.runAllTimersAsync();
      await flushMicrotasks();
      transcript = await reviewPromise;
    });

    expect(transcript).toBe("transcribed text");
    expect(stopPlaybackMock).toHaveBeenCalledTimes(1);
    expect(vi.mocked(transcribeAudio)).toHaveBeenCalledWith(expect.any(Blob), "en", "AP-123");
    expect(result.current.voiceRecorderState).toBe("review_ready");
  });

  it("recovers a stale warm stream on focus and reuses the new stream on the next start", async () => {
    const first = createStream();
    const second = createStream();
    getUserMediaMock
      .mockResolvedValueOnce(first.stream)
      .mockResolvedValueOnce(second.stream);

    const { result } = renderHook(() => useVoiceRecorder({
      lang: "en",
      isActive: true,
      shouldKeepMicWarm: true,
    }));

    await act(async () => {
      await result.current.prepareMicrophone();
      await flushMicrotasks();
    });

    act(() => {
      first.tracks[0].readyState = "ended";
      first.tracks[0].emitEnded();
    });

    expect(result.current.micWarmState).toBe("recovering");

    await act(async () => {
      window.dispatchEvent(new Event("focus"));
      await flushMicrotasks();
    });

    expect(getUserMediaMock).toHaveBeenCalledTimes(2);
    expect(result.current.micWarmState).toBe("warm");

    await act(async () => {
      await result.current.startVoiceRecording();
    });

    expect(recorderStartMock).toHaveBeenCalledTimes(1);
    expect(first.tracks[0].stop).not.toHaveBeenCalled();
  });

  it("invalidates the warm stream on recorder error and reacquires on explicit prepare", async () => {
    const first = createStream();
    const second = createStream();
    getUserMediaMock
      .mockResolvedValueOnce(first.stream)
      .mockResolvedValueOnce(second.stream);

    const { result } = renderHook(() => useVoiceRecorder({
      lang: "en",
      isActive: true,
      shouldKeepMicWarm: true,
    }));

    await act(async () => {
      await result.current.prepareMicrophone();
      await flushMicrotasks();
      await result.current.startVoiceRecording();
    });

    act(() => {
      MockMediaRecorder.instances[0]?.emitError();
    });

    expect(first.tracks[0].stop).toHaveBeenCalledTimes(1);
    expect(result.current.micWarmState).toBe("error");

    await act(async () => {
      expect(await result.current.prepareMicrophone()).toBe(true);
      await flushMicrotasks();
    });

    expect(getUserMediaMock).toHaveBeenCalledTimes(2);
    expect(result.current.micWarmState).toBe("warm");
  });
});
