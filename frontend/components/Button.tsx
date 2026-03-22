"use client";

import type { ButtonHTMLAttributes } from "react";

type Variant = "primary" | "secondary" | "danger";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  fullWidth?: boolean;
}

const variantClasses: Record<Variant, string> = {
  primary:
    "bg-primary text-white hover:bg-primary-dark active:bg-primary-dark focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2",
  secondary:
    "bg-white text-primary border border-primary hover:bg-base-lightest active:bg-base-lighter focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2",
  danger:
    "bg-error text-white hover:opacity-90 active:opacity-80 focus:outline-none focus:ring-2 focus:ring-error focus:ring-offset-2",
};

export function Button({
  variant = "primary",
  fullWidth = false,
  className = "",
  children,
  disabled,
  ...props
}: ButtonProps) {
  return (
    <button
      {...props}
      disabled={disabled}
      className={[
        "inline-flex items-center justify-center",
        "px-5 py-3",
        "text-base font-semibold",
        "rounded-btn",
        "transition-colors duration-150",
        "min-h-[44px]", // accessible touch target
        fullWidth ? "w-full" : "",
        disabled ? "opacity-50 cursor-not-allowed" : "cursor-pointer",
        variantClasses[variant],
        className,
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {children}
    </button>
  );
}
