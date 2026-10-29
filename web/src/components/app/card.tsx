import type { ReactNode } from "react";
import { cn } from "~/lib/utils";

export function Card({ children, className }: { children: ReactNode; className?: string }) {
  return <section className={cn("rounded-xl border border-line bg-surface", className)}>{children}</section>;
}

export function CardHeader({
  title,
  description,
  actions,
  id,
}: {
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
  id?: string;
}) {
  return (
    <div className="flex min-h-12 flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-3">
      <div className="min-w-0">
        <h2 id={id} className="text-base font-bold">
          {title}
        </h2>
        {description ? <p className="mt-0.5 text-sm text-muted">{description}</p> : null}
      </div>
      {actions ? <div className="flex items-center gap-2">{actions}</div> : null}
    </div>
  );
}

export function CardBody({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("p-5", className)}>{children}</div>;
}
