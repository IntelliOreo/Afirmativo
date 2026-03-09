"use client";

import { Button } from "@components/Button";
import type { Lang } from "@/lib/language";

interface TextAnswerPanelProps {
  lang: Lang;
  textAnswer: string;
  textAnswerCharCount: number;
  maxChars: number;
  onTextAnswerChange: (nextValue: string) => void;
  onSubmitAnswer: () => void | Promise<void>;
}

export function TextAnswerPanel({
  lang,
  textAnswer,
  textAnswerCharCount,
  maxChars,
  onTextAnswerChange,
  onSubmitAnswer,
}: TextAnswerPanelProps) {
  return (
    <>
      <div className="mb-4">
        <label className="block font-semibold text-primary-darkest mb-2">
          {lang === "es" ? "Su respuesta" : "Your answer"}
          <span className="block text-sm font-normal text-gray-500">
            {lang === "es"
              ? "Responda en su idioma seleccionado"
              : "Please answer in your selected language"}
          </span>
        </label>
        <textarea
          value={textAnswer}
          onChange={(e) => onTextAnswerChange(e.target.value.slice(0, maxChars))}
          maxLength={maxChars}
          rows={6}
          className="w-full px-3 py-3 text-base border border-base-lighter rounded focus:outline-none focus:ring-2 focus:ring-primary resize-none"
          placeholder={
            lang === "es"
              ? "Escriba su respuesta aquí..."
              : "Type your answer here..."
          }
        />
        <p className="mt-2 text-right text-sm text-primary-darkest">
          {lang === "es" ? "Caracteres" : "Characters"}: {textAnswerCharCount} / {maxChars}
        </p>
      </div>

      <Button fullWidth disabled={!textAnswer.trim()} onClick={() => { void onSubmitAnswer(); }}>
        {lang === "es" ? "Enviar respuesta" : "Submit answer"}
      </Button>
    </>
  );
}
