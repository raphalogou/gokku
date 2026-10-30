import { Search } from "lucide-react";
import { useId } from "react";
import { cn } from "~/lib/utils";
import { Input } from "./input";

/** A search box with a magnifier and a visually hidden label. */
export function SearchInput({
  label,
  value,
  onChange,
  className,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  className?: string;
}) {
  const id = useId();
  return (
    <div className={cn("relative w-full sm:w-[240px]", className)}>
      <label htmlFor={id} className="sr-only">
        {label}
      </label>
      <Search className="pointer-events-none absolute top-3 left-3 size-[18px] text-muted" aria-hidden />
      <Input
        id={id}
        type="search"
        placeholder={label}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="pl-10"
      />
    </div>
  );
}
