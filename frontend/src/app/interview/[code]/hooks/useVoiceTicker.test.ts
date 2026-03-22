import { renderHook } from "@testing-library/react";
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useVoiceTicker } from "./useVoiceTicker";

describe("useVoiceTicker", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-03-10T12:00:00Z"));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("updates duration on second boundaries with a 1000ms tick interval", async () => {
    const onLimitReached = vi.fn();
    const { result } = renderHook(() => useVoiceTicker({
      maxSeconds: 3,
      warningMilestones: [2],
      onLimitReached,
      tickIntervalMs: 1000,
    }));

    act(() => {
      result.current.start();
    });

    expect(result.current.durationSeconds).toBe(0);
    expect(result.current.warningSeconds).toBeNull();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(999);
    });
    expect(result.current.durationSeconds).toBe(0);
    expect(result.current.warningSeconds).toBeNull();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(result.current.durationSeconds).toBe(1);
    expect(result.current.warningSeconds).toBeNull();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(result.current.durationSeconds).toBe(2);
    expect(result.current.warningSeconds).toBe(2);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(result.current.durationSeconds).toBe(3);
    expect(onLimitReached).toHaveBeenCalledTimes(1);
  });
});
