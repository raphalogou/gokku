import { useEffect, useState } from "react";
import { getStreamAppLogsUrl } from "~/lib/api/gen/client";
import type { LogLine } from "~/lib/api/gen/model";
import { type ConsoleLine, clockTime, detectLevel } from "~/lib/console";

let counter = 0;

export function toLogConsole(l: LogLine): ConsoleLine {
  const { level, text } = detectLevel(l.text);
  counter += 1;
  return { key: `l${counter}`, time: clockTime(l.ts), label: l.process ?? "", level, text };
}

export type LogStreamState = "off" | "connecting" | "live" | "error";

/**
 * Follows an app's output over SSE while enabled (LOG-2). The server runs one
 * `dokku logs --tail` per stream and stops it when the stream closes (LOG-4).
 */
export function useLogStream(
  app: string,
  process: string | undefined,
  enabled: boolean,
  push: (lines: ConsoleLine[]) => void,
): LogStreamState {
  const [state, setState] = useState<LogStreamState>("off");

  useEffect(() => {
    if (!enabled) {
      setState("off");
      return;
    }
    setState("connecting");
    const source = new EventSource(getStreamAppLogsUrl(app, process ? { process } : undefined));
    source.onopen = () => setState("live");
    source.onerror = () => setState(source.readyState === EventSource.CLOSED ? "error" : "connecting");
    source.addEventListener("line", (e) => {
      push([toLogConsole(JSON.parse((e as MessageEvent<string>).data) as LogLine)]);
    });
    return () => source.close();
  }, [app, process, enabled, push]);

  return state;
}
