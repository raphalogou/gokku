import { Download, Pause, Play } from "lucide-react";
import { useEffect, useRef } from "react";
import { type ConsoleLine, consoleText, type Level } from "~/lib/console";
import { strings } from "~/lib/strings";
import { cn } from "~/lib/utils";
import { Button } from "../ui/button";

const levelClass: Record<Level, string> = {
  ERROR: "font-bold text-failed-fg",
  WARN: "font-bold text-deploying-fg",
  INFO: "text-muted",
  DEBUG: "text-muted",
};

/**
 * Monospace output with a time, a label (process or step), the level word as text (LOG-5)
 * and the line. While following, it keeps the newest line in view.
 */
export function Console({
  lines,
  label,
  follow,
  empty,
  className,
}: {
  lines: ConsoleLine[];
  label: string;
  follow: boolean;
  empty: string;
  className?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const labelWidth = Math.min(12, Math.max(4, ...lines.map((l) => l.label?.length ?? 0)));

  // biome-ignore lint/correctness/useExhaustiveDependencies: scroll whenever new lines arrive
  useEffect(() => {
    const el = ref.current;
    if (follow && el) el.scrollTop = el.scrollHeight;
  }, [lines, follow]);

  return (
    <div
      ref={ref}
      role="log"
      aria-label={label}
      // biome-ignore lint/a11y/noNoninteractiveTabindex: the scrollable log must be reachable by keyboard
      tabIndex={0}
      className={cn(
        "h-[480px] overflow-auto rounded-lg bg-code-bg px-4 py-3.5 font-mono text-[13px] leading-[1.75]",
        className,
      )}
    >
      {lines.length === 0 ? (
        <p className="font-sans text-muted">{empty}</p>
      ) : (
        lines.map((l) => (
          <div key={l.key} className="whitespace-pre">
            {l.time ? <span className="text-muted">{l.time} </span> : null}
            {l.label != null ? (
              <span className="font-medium">{(l.label ?? "").padEnd(labelWidth)} </span>
            ) : null}
            <span className={levelClass[l.level]}>{l.level.padEnd(5)}</span>
            {"  "}
            {l.text}
          </div>
        ))
      )}
    </div>
  );
}

/** "Following" or "Paused", the pause toggle and the download button under a console. */
export function ConsoleControls({
  lines,
  paused,
  waiting,
  onPausedChange,
  live,
  filename,
}: {
  lines: ConsoleLine[];
  paused: boolean;
  waiting: number;
  onPausedChange: (paused: boolean) => void;
  live: boolean;
  filename: string;
}) {
  const download = () => {
    const url = URL.createObjectURL(new Blob([consoleText(lines)], { type: "text/plain" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  const t = strings.console;
  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <span aria-live="polite" className="inline-flex items-center gap-1.5 text-[13px] font-semibold">
        {live ? (
          paused ? (
            <>
              <span className="size-2 rounded-full bg-deploying-fg" aria-hidden />
              <span className="text-deploying-fg">{t.paused(waiting)}</span>
            </>
          ) : (
            <>
              <span className="size-2 rounded-full bg-running-fg" aria-hidden />
              <span className="text-running-fg">{t.following}</span>
            </>
          )
        ) : (
          <span className="text-muted">{t.ended}</span>
        )}
      </span>
      <div className="flex gap-2">
        {live ? (
          <Button variant="secondary" onClick={() => onPausedChange(!paused)} aria-pressed={paused}>
            {paused ? <Play aria-hidden /> : <Pause aria-hidden />}
            {paused ? t.resume : t.pause}
          </Button>
        ) : null}
        <Button variant="secondary" onClick={download} disabled={lines.length === 0}>
          <Download aria-hidden />
          {t.download}
        </Button>
      </div>
    </div>
  );
}
