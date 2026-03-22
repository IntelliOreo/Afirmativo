import { log } from "@/lib/logger";
import type { Dispatch, MutableRefObject, SetStateAction } from "react";
import type { Lang } from "@/lib/language";
import { VoiceTranscriptionError, transcribeAudio } from "./voiceTranscription";
import type { VoiceFeedback, VoiceRecorderState } from "../viewTypes";

interface CreateReviewVoiceRecordingDeps {
  isActive: boolean;
  voiceRecorderState: VoiceRecorderState;
  voiceBlobRef: MutableRefObject<Blob | null>;
  langRef: MutableRefObject<Lang>;
  stopPlayback: () => void;
  setVoiceRecorderState: Dispatch<SetStateAction<VoiceRecorderState>>;
  setVoiceError: Dispatch<SetStateAction<VoiceFeedback | null>>;
  setVoiceInfo: Dispatch<SetStateAction<VoiceFeedback | null>>;
  resetAfterReviewFailure: () => void;
}

export function createReviewVoiceRecording({
  isActive,
  voiceRecorderState,
  voiceBlobRef,
  langRef,
  stopPlayback,
  setVoiceRecorderState,
  setVoiceError,
  setVoiceInfo,
  resetAfterReviewFailure,
}: CreateReviewVoiceRecordingDeps) {
  return async (sessionCode: string): Promise<string | null> => {
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
    setVoiceError(null);
    setVoiceInfo({ code: "preparing_transcript" });

    try {
      const transcript = await transcribeAudio(audioBlob, langRef.current, trimmedSessionCode);
      setVoiceRecorderState("review_ready");
      setVoiceInfo({ code: "transcript_ready" });
      log.info("voice review transcription completed", {
        phase: "review_done",
        transcript_length: transcript.length,
      });
      return transcript;
    } catch (err) {
      resetAfterReviewFailure();

      log.error("voice review transcription failed", {
        phase: "review_error",
        error: err instanceof Error ? err.message : "unknown_error",
      });
      if (err instanceof VoiceTranscriptionError) {
        if (err.reason === "session_unauthorized") {
          setVoiceError({ code: "session_unauthorized", requestId: err.requestId });
        } else if (err.reason === "voice_api_unconfigured" || err.reason === "token_unavailable") {
          setVoiceError({ code: "voice_api_unavailable", requestId: err.requestId });
        } else {
          setVoiceError({ code: "transcription_failed", requestId: err.requestId });
        }
      } else {
        setVoiceError({ code: "transcription_failed" });
      }
      return null;
    }
  };
}
