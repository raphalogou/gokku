import type { ComponentProps, ReactNode } from "react";
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

/**
 * An input with buttons inside its border, such as reveal and copy. The group shows the
 * focus ring of its input; each addon is a 44 px button with its own label.
 */
export function InputGroup({
  className,
  readOnly,
  invalid,
  children,
}: {
  className?: string;
  readOnly?: boolean;
  invalid?: boolean;
  children: ReactNode;
}) {
  return (
    <div
      className={cn(
        "flex min-h-11 w-full items-stretch overflow-hidden rounded-lg border border-input-border",
        "has-[input:focus-visible]:outline-2 has-[input:focus-visible]:outline-offset-2 has-[input:focus-visible]:outline-accent",
        readOnly ? "bg-subtle" : "bg-surface",
        invalid && "border-failed",
        className,
      )}
    >
      {children}
    </div>
  );
}

export function InputGroupInput({ className, ...props }: ComponentProps<"input">) {
  return (
    <input
      className={cn(
        "min-w-0 flex-1 bg-transparent px-3 text-[15px] text-ink placeholder:text-muted focus-visible:outline-none",
        "read-only:text-muted",
        className,
      )}
      {...props}
    />
  );
}

export function InputAddon({ className, type = "button", ...props }: ComponentProps<"button">) {
  return (
    <button
      type={type}
      className={cn(
        "flex w-11 shrink-0 cursor-pointer items-center justify-center text-muted hover:bg-subtle hover:text-ink",
        "focus-visible:-outline-offset-2 disabled:cursor-not-allowed disabled:opacity-50 [&_svg]:size-[18px]",
        className,
      )}
      {...props}
    />
  );
}
