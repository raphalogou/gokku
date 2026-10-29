import type { ComponentProps, ReactNode } from "react";
import { cn } from "~/lib/utils";

/** A table inside a card: header row on the subtle surface, rows at least 60 px, and
 * horizontal scrolling inside the card on narrow screens (DESIGN.md, 5.4). */
export function DataTable({
  label,
  className,
  children,
}: {
  label: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    // relative: visually hidden headers are position: absolute and must not widen the page.
    <div className="relative overflow-x-auto">
      <table aria-label={label} className={cn("w-full border-collapse text-left", className)}>
        {children}
      </table>
    </div>
  );
}

export function THead({ children }: { children: ReactNode }) {
  return (
    <thead className="bg-subtle">
      <tr className="border-b border-line">{children}</tr>
    </thead>
  );
}

export function TH({ className, ...props }: ComponentProps<"th">) {
  return (
    <th
      scope="col"
      className={cn("px-5 py-2.5 text-[13px] font-semibold text-muted", className)}
      {...props}
    />
  );
}

export function TR({ className, ...props }: ComponentProps<"tr">) {
  return <tr className={cn("h-[60px] border-b border-line last:border-b-0", className)} {...props} />;
}

export function TD({ className, ...props }: ComponentProps<"td">) {
  return <td className={cn("px-5 py-2 align-middle", className)} {...props} />;
}
