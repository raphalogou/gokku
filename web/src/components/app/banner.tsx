import { CircleAlert, CircleCheck, Info } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "~/lib/utils";

const variants = {
  info: { icon: Info, className: "border-line bg-subtle text-ink" },
  success: { icon: CircleCheck, className: "border-running-fg/30 bg-running-bg text-running-fg" },
  error: { icon: CircleAlert, className: "border-failed/40 bg-failed-bg text-failed-fg" },
};

export function Banner({
  variant = "info",
  children,
  className,
}: {
  variant?: keyof typeof variants;
  children: ReactNode;
  className?: string;
}) {
  const { icon: Icon, className: tone } = variants[variant];
  return (
    <div
      role={variant === "error" ? "alert" : "status"}
      className={cn("flex items-start gap-3 rounded-lg border px-4 py-3 text-[15px]", tone, className)}
    >
      <Icon className="mt-0.5 size-[18px] shrink-0" aria-hidden />
      <div className="min-w-0">{children}</div>
    </div>
  );
}
