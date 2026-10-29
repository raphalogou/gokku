import { Select as BaseSelect } from "@base-ui/react/select";
import { Check, ChevronDown } from "lucide-react";
import { cn } from "~/lib/utils";

type Option<T extends string> = { value: T; label: string; hint?: string };

type SelectProps<T extends string> = {
  value: T;
  onValueChange: (value: T) => void;
  options: Option<T>[];
  id?: string;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
  "aria-label"?: string;
  className?: string;
};

export function Select<T extends string>({
  value,
  onValueChange,
  options,
  className,
  ...aria
}: SelectProps<T>) {
  return (
    <BaseSelect.Root value={value} onValueChange={(v) => v !== null && onValueChange(v as T)} items={options}>
      <BaseSelect.Trigger
        {...aria}
        className={cn(
          "flex min-h-11 w-full cursor-pointer items-center justify-between gap-2 rounded-lg border border-input-border bg-surface px-3 text-left text-[15px]",
          className,
        )}
      >
        <BaseSelect.Value />
        <BaseSelect.Icon>
          <ChevronDown className="size-[18px] text-muted" aria-hidden />
        </BaseSelect.Icon>
      </BaseSelect.Trigger>
      <BaseSelect.Portal>
        <BaseSelect.Positioner sideOffset={6} className="z-50 outline-none">
          <BaseSelect.Popup className="min-w-(--anchor-width) rounded-lg border border-line bg-surface p-1 shadow-lg">
            <BaseSelect.List>
              {options.map((o) => (
                <BaseSelect.Item
                  key={o.value}
                  value={o.value}
                  className="flex min-h-11 cursor-pointer items-start gap-2 rounded-md px-2 py-2 outline-none select-none data-highlighted:bg-subtle"
                >
                  <span className="mt-0.5 size-[18px] shrink-0">
                    <BaseSelect.ItemIndicator>
                      <Check className="size-[18px] text-accent" aria-hidden />
                    </BaseSelect.ItemIndicator>
                  </span>
                  <span className="flex flex-col">
                    <BaseSelect.ItemText>{o.label}</BaseSelect.ItemText>
                    {o.hint ? <span className="text-[13px] text-muted">{o.hint}</span> : null}
                  </span>
                </BaseSelect.Item>
              ))}
            </BaseSelect.List>
          </BaseSelect.Popup>
        </BaseSelect.Positioner>
      </BaseSelect.Portal>
    </BaseSelect.Root>
  );
}
