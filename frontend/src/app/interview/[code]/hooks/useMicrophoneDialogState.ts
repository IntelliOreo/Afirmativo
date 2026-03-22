"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { Question } from "../models";
import type {
  MicrophoneWarmupDialogMode,
  MicrophoneWarmupDialogState,
  MicWarmState,
} from "../viewTypes";

const MIC_RECONNECT_DIALOG_REVEAL_MS = 300;
const MIC_SUCCESS_HANDOFF_DELAY_MS = 500;

interface UseMicrophoneDialogStateParams {
  micWarmState: MicWarmState;
  prepareMicrophone: () => Promise<boolean>;
  currentQuestion: Question;
  phase: "active" | "submitting";
  hasMicOptIn: boolean;
  onMicOptIn: () => void;
}

interface UseMicrophoneDialogStateResult {
  showMicrophoneDialog: boolean;
  activeMicDialogMode: MicrophoneWarmupDialogMode | null;
  micDialogUiState: MicrophoneWarmupDialogState;
  handleEnableMicrophone: () => Promise<void>;
  handleDismissMicrophonePrompt: () => void;
}

export function useMicrophoneDialogState({
  micWarmState,
  prepareMicrophone,
  currentQuestion,
  phase,
  hasMicOptIn,
  onMicOptIn,
}: UseMicrophoneDialogStateParams): UseMicrophoneDialogStateResult {
  const [hasSeenMicReadinessPrompt, setHasSeenMicReadinessPrompt] = useState(false);
  const [activeMicDialogMode, setActiveMicDialogMode] = useState<MicrophoneWarmupDialogMode | null>(null);
  const [micDialogUiState, setMicDialogUiState] = useState<MicrophoneWarmupDialogState>("idle");
  const [suppressReconnectDialog, setSuppressReconnectDialog] = useState(false);
  const reconnectDialogTimerRef = useRef<number | null>(null);
  const micSuccessHandoffTimerRef = useRef<number | null>(null);

  const clearReconnectDialogTimer = useCallback(() => {
    if (reconnectDialogTimerRef.current == null) return;
    window.clearTimeout(reconnectDialogTimerRef.current);
    reconnectDialogTimerRef.current = null;
  }, []);

  const clearMicSuccessHandoffTimer = useCallback(() => {
    if (micSuccessHandoffTimerRef.current == null) return;
    window.clearTimeout(micSuccessHandoffTimerRef.current);
    micSuccessHandoffTimerRef.current = null;
  }, []);

  const closeMicrophoneDialog = useCallback(() => {
    clearReconnectDialogTimer();
    clearMicSuccessHandoffTimer();
    setActiveMicDialogMode(null);
    setMicDialogUiState("idle");
  }, [clearMicSuccessHandoffTimer, clearReconnectDialogTimer]);

  const beginMicSuccessHandoff = useCallback((mode: MicrophoneWarmupDialogMode) => {
    clearReconnectDialogTimer();
    clearMicSuccessHandoffTimer();
    setActiveMicDialogMode(mode);
    setMicDialogUiState("ready_handoff");
    micSuccessHandoffTimerRef.current = window.setTimeout(() => {
      if (mode === "initial_setup") {
        onMicOptIn();
        setHasSeenMicReadinessPrompt(true);
      }
      setSuppressReconnectDialog(false);
      setActiveMicDialogMode(null);
      setMicDialogUiState("idle");
      micSuccessHandoffTimerRef.current = null;
    }, MIC_SUCCESS_HANDOFF_DELAY_MS);
  }, [clearMicSuccessHandoffTimer, clearReconnectDialogTimer, onMicOptIn]);

  const handleEnableMicrophone = useCallback(async () => {
    const nextDialogMode = activeMicDialogMode ?? (hasMicOptIn ? "reconnect" : "initial_setup");
    clearReconnectDialogTimer();
    clearMicSuccessHandoffTimer();
    setSuppressReconnectDialog(false);
    setActiveMicDialogMode(nextDialogMode);
    setMicDialogUiState(micWarmState === "recovering" ? "recovering" : "warming");
    const prepared = await prepareMicrophone();
    if (!prepared) return;
    beginMicSuccessHandoff(nextDialogMode);
  }, [
    activeMicDialogMode,
    beginMicSuccessHandoff,
    clearMicSuccessHandoffTimer,
    clearReconnectDialogTimer,
    hasMicOptIn,
    micWarmState,
    prepareMicrophone,
  ]);

  const handleDismissMicrophonePrompt = useCallback(() => {
    if (activeMicDialogMode === "initial_setup") {
      setHasSeenMicReadinessPrompt(true);
    } else if (activeMicDialogMode === "reconnect") {
      setSuppressReconnectDialog(true);
    }
    closeMicrophoneDialog();
  }, [activeMicDialogMode, closeMicrophoneDialog]);

  useEffect(() => {
    return () => {
      clearReconnectDialogTimer();
      clearMicSuccessHandoffTimer();
    };
  }, [clearMicSuccessHandoffTimer, clearReconnectDialogTimer]);

  useEffect(() => {
    if (activeMicDialogMode !== null || phase !== "active") return;
    if (currentQuestion.kind !== "readiness" || hasMicOptIn || hasSeenMicReadinessPrompt) return;
    setActiveMicDialogMode("initial_setup");
    setMicDialogUiState("idle");
  }, [activeMicDialogMode, currentQuestion.kind, hasMicOptIn, hasSeenMicReadinessPrompt, phase]);

  useEffect(() => {
    if (activeMicDialogMode === "initial_setup") {
      if (micDialogUiState === "ready_handoff") return;
      if (micWarmState === "warming") {
        setMicDialogUiState("warming");
      } else if (micWarmState === "recovering") {
        setMicDialogUiState("recovering");
      } else if (micWarmState === "denied") {
        setMicDialogUiState("denied");
      } else if (micWarmState === "error") {
        setMicDialogUiState("error");
      }
      return;
    }

    if (!hasMicOptIn) {
      clearReconnectDialogTimer();
      if (activeMicDialogMode === "reconnect") {
        closeMicrophoneDialog();
      }
      return;
    }

    if (micWarmState === "warm") {
      clearReconnectDialogTimer();
      if (
        activeMicDialogMode === "reconnect"
        && (micDialogUiState === "warming" || micDialogUiState === "recovering")
      ) {
        beginMicSuccessHandoff("reconnect");
      }
      setSuppressReconnectDialog(false);
      return;
    }

    if (micWarmState === "cold") {
      clearReconnectDialogTimer();
      setSuppressReconnectDialog(false);
      if (activeMicDialogMode === "reconnect") {
        closeMicrophoneDialog();
      }
      return;
    }

    if (micWarmState === "recovering") {
      if (activeMicDialogMode === "reconnect") {
        setMicDialogUiState("recovering");
        return;
      }
      if (suppressReconnectDialog) return;
      clearReconnectDialogTimer();
      reconnectDialogTimerRef.current = window.setTimeout(() => {
        setActiveMicDialogMode("reconnect");
        setMicDialogUiState("recovering");
        reconnectDialogTimerRef.current = null;
      }, MIC_RECONNECT_DIALOG_REVEAL_MS);
      return;
    }

    clearReconnectDialogTimer();
    if (micWarmState === "denied" || micWarmState === "error") {
      if (suppressReconnectDialog) return;
      setActiveMicDialogMode("reconnect");
      setMicDialogUiState(micWarmState);
    }
  }, [
    activeMicDialogMode,
    beginMicSuccessHandoff,
    clearReconnectDialogTimer,
    closeMicrophoneDialog,
    hasMicOptIn,
    micDialogUiState,
    micWarmState,
    suppressReconnectDialog,
  ]);

  return {
    showMicrophoneDialog: activeMicDialogMode !== null,
    activeMicDialogMode,
    micDialogUiState,
    handleEnableMicrophone,
    handleDismissMicrophonePrompt,
  };
}
