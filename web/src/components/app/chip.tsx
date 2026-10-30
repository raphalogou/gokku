import type { ReactNode } from "react";
import { cn } from "~/lib/utils";

/** A monospace label for app and service names (DESIGN.md, 5.4). */
export function Chip({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <span
      className={cn(
        "inline-block rounded-md bg-chip px-2 py-0.5 font-mono text-[12px] whitespace-nowrap text-ink",
        className,
      )}
    >
      {children}
    </span>
  );
}

/** A wrapping list of chips, or a muted fallback when there are none. */
export function ChipList({ items, empty }: { items: string[]; empty: string }) {
  if (items.length === 0) return <span className="text-sm text-muted">{empty}</span>;
  return (
    <span className="flex flex-wrap gap-1.5">
      {items.map((item, i) => (
        // Items can repeat, for example two services of one type.
        // biome-ignore lint/suspicious/noArrayIndexKey: the list is static for a render
        <Chip key={`${item}-${i}`}>{item}</Chip>
      ))}
    </span>
  );
}
