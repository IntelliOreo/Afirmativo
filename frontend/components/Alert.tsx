import type { HTMLAttributes, ReactNode } from "react";

type AlertVariant = "info" | "warning" | "error" | "success";

interface AlertProps extends HTMLAttributes<HTMLDivElement> {
  variant?: AlertVariant;
  children: ReactNode;
}

const variantStyles: Record<
  AlertVariant,
  { border: string; bg: string }
> = {
  info: {
    border: "border-l-4 border-accent-cool",
    bg: "bg-blue-50",
  },
  warning: {
    border: "border-l-4 border-accent-warm",
    bg: "bg-orange-50",
  },
  error: {
    border: "border-l-4 border-error",
    bg: "bg-red-50",
  },
  success: {
    border: "border-l-4 border-success",
    bg: "bg-green-50",
  },
};

export function Alert({
  variant = "info",
  children,
  className = "",
  ...props
}: AlertProps) {
  const styles = variantStyles[variant];

  return (
    <div
      role="alert"
      {...props}
      className={[
        styles.border,
        styles.bg,
        "px-4 py-3",
        "rounded-r",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
    >
      <div className="text-primary-darkest">{children}</div>
    </div>
  );
}
