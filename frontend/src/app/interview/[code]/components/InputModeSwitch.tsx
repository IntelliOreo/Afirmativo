"use client";

import { memo } from "react";
import { Button } from "@components/Button";
import type { Lang } from "@/lib/language";
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
  return (
    <div className="mb-6 grid grid-cols-1 gap-3 sm:grid-cols-2">
      <Button
        type="button"
        variant={inputMode === "text" ? "primary" : "secondary"}
        disabled={!canSwitchModes}
        onClick={onSelectText}
      >
        {lang === "es" ? "Entrada por texto" : "Text input"}
      </Button>
      <Button
        type="button"
        variant={inputMode === "voice" ? "primary" : "secondary"}
        disabled={!canSwitchModes}
        onClick={onSelectVoice}
      >
        {lang === "es" ? "Entrada por voz" : "Voice input"}
      </Button>
    </div>
  );
});
