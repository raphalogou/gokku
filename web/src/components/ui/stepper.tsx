import { Minus, Plus } from "lucide-react";
import { Button } from "./button";

/** A number input between minus and plus buttons, each 44 px (DESIGN 5.4). */
export function Stepper({
  label,
  value,
  onChange,
  min = 0,
  max = 100,
  disabled,
}: {
  label: string;
  value: number;
  onChange: (value: number) => void;
  min?: number;
  max?: number;
  disabled?: boolean;
}) {
  const clamp = (n: number) => Math.min(max, Math.max(min, Number.isFinite(n) ? Math.round(n) : min));
  return (
    <div className="inline-flex items-center gap-2">
      <Button
        variant="secondary"
        size="icon"
        aria-label={`Decrease ${label}`}
        onClick={() => onChange(clamp(value - 1))}
        disabled={disabled || value <= min}
      >
        <Minus aria-hidden />
      </Button>
      <input
        type="number"
        inputMode="numeric"
        aria-label={label}
        min={min}
        max={max}
        value={value}
        disabled={disabled}
        onChange={(e) => onChange(clamp(e.target.valueAsNumber))}
        className="h-11 w-16 rounded-lg border border-input-border bg-surface text-center font-mono text-[15px] disabled:opacity-60"
      />
      <Button
        variant="secondary"
        size="icon"
        aria-label={`Increase ${label}`}
        onClick={() => onChange(clamp(value + 1))}
        disabled={disabled || value >= max}
      >
        <Plus aria-hidden />
      </Button>
    </div>
  );
}
