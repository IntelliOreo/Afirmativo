"use client";

import type { InputHTMLAttributes } from "react";

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
  labelEs?: string;
  error?: string;
  hint?: string;
}

export function Input({
  label,
  labelEs,
  error,
  hint,
  id,
  className = "",
  ...props
}: InputProps) {
  const inputId = id ?? label.toLowerCase().replace(/\s+/g, "-");

  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={inputId} className="font-semibold text-primary-darkest">
        {label}
        {labelEs && (
          <span className="block text-sm font-normal text-gray-600">
            {labelEs}
          </span>
        )}
      </label>
      {hint && <p className="text-sm text-gray-600">{hint}</p>}
      <input
        {...props}
        id={inputId}
        aria-invalid={!!error}
        aria-describedby={error ? `${inputId}-error` : undefined}
        className={[
          "w-full",
          "px-3 py-3",
          "text-base",
          "border rounded",
          "min-h-[44px]",
          "focus:outline-none focus:ring-2 focus:ring-primary focus:border-primary",
          error
            ? "border-error focus:ring-error"
            : "border-base-lighter",
          className,
        ]
          .filter(Boolean)
          .join(" ")}
      />
      {error && (
        <p id={`${inputId}-error`} className="text-sm text-error font-semibold">
          {error}
        </p>
      )}
    </div>
  );
}
