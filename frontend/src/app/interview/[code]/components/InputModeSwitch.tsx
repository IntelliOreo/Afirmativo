"use client";

import { memo } from "react";
import { Button } from "@components/Button";
import type { Lang } from "@/lib/language";
import { getInterviewMessages } from "../messages/interviewMessages";
import type { InputMode } from "../viewTypes";

interface InputModeSwitchProps {
  lang: Lang;
  inputMode: InputMode;
  canSwitchModes: boolean;
  onSelectText: () => void;
  onSelectVoice: () => void;
}

export const InputModeSwitch = memo(function InputModeSwitch({
  lang,
  inputMode,
  canSwitchModes,
  onSelectText,
  onSelectVoice,
}: InputModeSwitchProps) {
  const t = getInterviewMessages(lang).inputMode;

  return (
    <div className="mb-6 grid grid-cols-1 gap-3 sm:grid-cols-2">
      <Button
        type="button"
        variant={inputMode === "text" ? "primary" : "secondary"}
        disabled={!canSwitchModes}
        onClick={onSelectText}
      >
        {t.text}
      </Button>
      <Button
        type="button"
        variant={inputMode === "voice" ? "primary" : "secondary"}
        disabled={!canSwitchModes}
        onClick={onSelectVoice}
      >
        {t.voice}
      </Button>
    </div>
  );
});
