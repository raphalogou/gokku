import type { ComponentProps } from "react";
import { cn } from "~/lib/utils";

export function Input({ className, ...props }: ComponentProps<"input">) {
  return (
    <input
      className={cn(
        "min-h-11 w-full rounded-lg border border-input-border bg-surface px-3 text-[15px] text-ink placeholder:text-muted",
        "read-only:bg-subtle read-only:text-muted aria-invalid:border-failed",
        className,
      )}
      {...props}
    />
  );
}

export function Textarea({ className, ...props }: ComponentProps<"textarea">) {
  return (
    <textarea
      className={cn(
        "min-h-24 w-full rounded-lg border border-input-border bg-surface px-3 py-2.5 text-[15px] text-ink placeholder:text-muted",
        "aria-invalid:border-failed",
        className,
      )}
      {...props}
    />
  );
}
