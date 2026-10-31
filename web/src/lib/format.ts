const relative = new Intl.RelativeTimeFormat("en", { numeric: "auto" });
const absolute = new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "short" });

const steps: [Intl.RelativeTimeFormatUnit, number][] = [
  ["year", 365 * 24 * 3600],
  ["month", 30 * 24 * 3600],
  ["week", 7 * 24 * 3600],
  ["day", 24 * 3600],
  ["hour", 3600],
  ["minute", 60],
];

/** "3 minutes ago", "in 6 days", "just now". */
export function timeAgo(iso: string, now = Date.now()): string {
  const seconds = Math.round((new Date(iso).getTime() - now) / 1000);
  for (const [unit, size] of steps) {
    if (Math.abs(seconds) >= size) return relative.format(Math.round(seconds / size), unit);
  }
  return "just now";
}

/** "Oct 29, 2026, 9:58 PM" in the viewer's time zone. */
export function dateTime(iso: string): string {
  return absolute.format(new Date(iso));
}

/** A short, readable name for a browser user agent: "Firefox on Linux". */
export function describeUserAgent(ua: string): string {
  if (!ua) return "Unknown device";
  const browser =
    [
      ["Edg/", "Edge"],
      ["Firefox/", "Firefox"],
      ["Chrome/", "Chrome"],
      ["Safari/", "Safari"],
      ["curl/", "curl"],
    ].find(([needle]) => ua.includes(needle as string))?.[1] ?? "Browser";
  const os =
    [
      ["Android", "Android"],
      ["iPhone", "iOS"],
      ["iPad", "iPadOS"],
      ["Mac OS X", "macOS"],
      ["Windows", "Windows"],
      ["Linux", "Linux"],
    ].find(([needle]) => ua.includes(needle as string))?.[1] ?? "";
  return os ? `${browser} on ${os}` : browser;
}

/** "41s", "1m 38s", "2h 5m" between two instants (the second defaults to now). */
export function duration(fromIso: string, toIso?: string | null, now = Date.now()): string {
  const ms = (toIso ? new Date(toIso).getTime() : now) - new Date(fromIso).getTime();
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}
