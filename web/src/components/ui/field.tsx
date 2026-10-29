import { cloneElement, type ReactElement, useId } from "react";
import { cn } from "~/lib/utils";

type FieldProps = {
  label: string;
  hint?: string;
  error?: string;
  className?: string;
  /** The control. It receives id, aria-invalid and aria-describedby. */
  children: ReactElement<{ id?: string; "aria-invalid"?: boolean; "aria-describedby"?: string }>;
};

/** A labelled form control with an optional hint and error, wired for screen readers. */
export function Field({ label, hint, error, className, children }: FieldProps) {
  const id = useId();
  // The error replaces the hint, so only the visible one describes the control.
  const errorId = error ? `${id}-error` : undefined;
  const hintId = hint && !error ? `${id}-hint` : undefined;
  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      <label htmlFor={id} className="text-sm font-semibold">
        {label}
      </label>
      {cloneElement(children, {
        id,
        "aria-invalid": error ? true : undefined,
        "aria-describedby": errorId ?? hintId,
      })}
      {error ? (
        <p id={errorId} className="text-sm text-failed">
          {error}
        </p>
      ) : hint ? (
        <p id={hintId} className="text-[13px] text-muted">
          {hint}
        </p>
      ) : null}
    </div>
  );
}
