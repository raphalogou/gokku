import type { ReactNode } from "react";
import { strings } from "~/lib/strings";
import { Button } from "../ui/button";

/** The unsaved changes count, discard, and the primary action (DESIGN 5.4). */
export function SaveBar({
  count,
  onDiscard,
  children,
  extra,
}: {
  count: number;
  onDiscard: () => void;
  children: ReactNode;
  extra?: ReactNode;
}) {
  if (count === 0) return null;
  return (
    <section
      aria-label={strings.saveBar.label}
      className="sticky bottom-20 z-20 mt-6 flex flex-wrap items-center justify-between gap-4 rounded-xl border border-line bg-surface px-5 py-3 shadow-md nav:bottom-4"
    >
      <span className="font-semibold" aria-live="polite">
        {strings.saveBar.unsaved(count)}
      </span>
      <div className="flex flex-wrap items-center gap-3">
        {extra}
        <Button variant="secondary" onClick={onDiscard}>
          {strings.saveBar.discard}
        </Button>
        {children}
      </div>
    </section>
  );
}
