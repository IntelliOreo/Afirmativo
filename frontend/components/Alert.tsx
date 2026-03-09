import type { HTMLAttributes, ReactNode } from "react";

type AlertVariant = "info" | "warning" | "error" | "success";

interface AlertProps extends HTMLAttributes<HTMLDivElement> {
  variant?: AlertVariant;
  title?: string;
  children: ReactNode;
}

const variantStyles: Record<
  AlertVariant,
  { border: string; bg: string; titleColor: string }
> = {
  info: {
    border: "border-l-4 border-accent-cool",
    bg: "bg-blue-50",
    titleColor: "text-primary-dark",
  },
  warning: {
    border: "border-l-4 border-accent-warm",
    bg: "bg-orange-50",
    titleColor: "text-primary-darkest",
  },
  error: {
    border: "border-l-4 border-error",
    bg: "bg-red-50",
    titleColor: "text-error",
  },
  success: {
    border: "border-l-4 border-success",
    bg: "bg-green-50",
    titleColor: "text-success",
  },
};

export function Alert({
  variant = "info",
  title,
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
      {title && (
        <p className={`font-semibold mb-1 ${styles.titleColor}`}>{title}</p>
      )}
      <div className="text-primary-darkest">{children}</div>
    </div>
  );
}
