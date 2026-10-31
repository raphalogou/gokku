/** Console lines shared by app logs and job output. */

export type Level = "ERROR" | "WARN" | "INFO" | "DEBUG";

export type ConsoleLine = {
  key: string;
  time?: string | null;
  label?: string | null;
  level: Level;
  text: string;
};

/** The most lines a console keeps in memory (DESIGN 5.2). */
export const maxConsoleLines = 5000;

const levelRe = /^\s*(?:\[)?(ERROR|ERR|FATAL|CRITICAL|WARN|WARNING|INFO|DEBUG|TRACE)(?:\])?:?\s+/i;
const anywhereRe = /\b(ERROR|FATAL|WARN(?:ING)?)\b/i;

const normalize: Record<string, Level> = {
  ERROR: "ERROR",
  ERR: "ERROR",
  FATAL: "ERROR",
  CRITICAL: "ERROR",
  WARN: "WARN",
  WARNING: "WARN",
  INFO: "INFO",
  DEBUG: "DEBUG",
  TRACE: "DEBUG",
};

/**
 * Guesses a line's level (LOG-5). It is cosmetic: a leading level word is moved into the
 * level column, otherwise ERROR or WARN anywhere in the line counts, and lines on stderr
 * that say nothing are warnings.
 */
export function detectLevel(text: string, stream?: string): { level: Level; text: string } {
  const lead = levelRe.exec(text);
  if (lead?.[1])
    return { level: normalize[lead[1].toUpperCase()] ?? "INFO", text: text.slice(lead[0].length) };
  const any = anywhereRe.exec(text);
  if (any?.[1]) return { level: normalize[any[1].toUpperCase()] ?? "INFO", text };
  return { level: stream === "stderr" ? "WARN" : "INFO", text };
}

/** Appends lines and keeps the newest maxConsoleLines. */
export function appendCapped(prev: ConsoleLine[], next: ConsoleLine[]): ConsoleLine[] {
  if (next.length === 0) return prev;
  const all = prev.concat(next);
  return all.length > maxConsoleLines ? all.slice(all.length - maxConsoleLines) : all;
}

const clock = new Intl.DateTimeFormat("en-GB", { hour: "2-digit", minute: "2-digit", second: "2-digit" });

/** "09:14:02" in the viewer's time zone. */
export function clockTime(iso: string | null | undefined): string {
  return iso ? clock.format(new Date(iso)) : "";
}

/** The console as plain text, for download (LOG-3). */
export function consoleText(lines: ConsoleLine[]): string {
  return lines
    .map((l) => [l.time ?? "", l.label ?? "", l.level, l.text].filter(Boolean).join("  "))
    .join("\n")
    .concat("\n");
}
