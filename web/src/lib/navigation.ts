/** Returns next if it is a same-site path, so a crafted ?next= cannot redirect off-site. */
export function safeNext(next: string | null | undefined, fallback = "/"): string {
  if (!next?.startsWith("/") || next.startsWith("//") || next.startsWith("/\\")) return fallback;
  return next;
}
