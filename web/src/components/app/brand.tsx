import { strings } from "~/lib/strings";

export function BrandMark({ className = "size-8" }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" className={className} aria-hidden>
      <rect width="32" height="32" rx="7" fill="var(--accent)" />
      <path
        d="M21.5 11.2A7 7 0 1 0 23 16h-6.5"
        fill="none"
        stroke="var(--on-accent)"
        strokeWidth="3"
        strokeLinecap="round"
      />
    </svg>
  );
}

export function Brand() {
  return (
    <span className="flex items-center gap-2.5 text-[17px] font-bold">
      <BrandMark />
      {strings.product}
    </span>
  );
}
