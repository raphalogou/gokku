import { Toggle } from "@base-ui/react/toggle";
import { ToggleGroup } from "@base-ui/react/toggle-group";
import { Monitor, Moon, Sun } from "lucide-react";
import { useSetTheme } from "~/hooks/use-theme";
import type { Theme } from "~/lib/api/gen/model";
import { strings } from "~/lib/strings";

const options = [
  { value: "system", label: strings.account.themeSystem, icon: Monitor },
  { value: "light", label: strings.account.themeLight, icon: Sun },
  { value: "dark", label: strings.account.themeDark, icon: Moon },
] as const;

/** Segmented control for the theme preference; each button reports aria-pressed. */
export function ThemeSwitcher({ value }: { value: Theme }) {
  const setTheme = useSetTheme();
  return (
    <div>
      <div id="theme-label" className="px-3 pt-2 pb-1.5 text-[13px] font-semibold text-muted">
        {strings.account.theme}
      </div>
      <ToggleGroup
        aria-labelledby="theme-label"
        value={[value]}
        onValueChange={(v) => {
          const next = v[0] as Theme | undefined;
          if (next && next !== value) setTheme.mutate(next);
        }}
        className="mx-1 mb-1 grid grid-cols-3 gap-1 rounded-lg bg-subtle p-1"
      >
        {options.map(({ value: v, label, icon: Icon }) => (
          <Toggle
            key={v}
            value={v}
            className="flex min-h-11 cursor-pointer flex-col items-center justify-center gap-0.5 rounded-md text-[13px] font-semibold text-muted data-pressed:bg-surface data-pressed:text-ink data-pressed:shadow-sm"
          >
            <Icon className="size-4" aria-hidden />
            {label}
          </Toggle>
        ))}
      </ToggleGroup>
    </div>
  );
}
