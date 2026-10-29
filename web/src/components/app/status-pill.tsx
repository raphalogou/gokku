import { cn } from "~/lib/utils";

export type StatusKind = "running" | "stopped" | "deploying" | "failed";

const styles: Record<StatusKind, string> = {
  running: "bg-running-bg text-running-fg",
  stopped: "bg-stopped-bg text-stopped-fg",
  deploying: "bg-deploying-bg text-deploying-fg",
  failed: "bg-failed-bg text-failed-fg",
};

/** A status with a dot and a text label. Status is never conveyed by color alone (NFR-4). */
export function StatusPill({
  kind,
  label,
  className,
}: {
  kind: StatusKind;
  label: string;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[13px] font-semibold whitespace-nowrap",
        styles[kind],
        className,
      )}
    >
      <span className="size-2 rounded-full bg-current" aria-hidden />
      {label}
    </span>
  );
}
