import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { queryKeys } from "~/lib/queries";

type EventName =
  | "app.updated"
  | "app.removed"
  | "service.updated"
  | "project.updated"
  | "job.updated"
  | "server.status";

/** The query keys each event makes stale (EVT-1). Lists embed related objects, so an app
 * change also touches projects and services. */
function invalidate(qc: QueryClient, event: EventName, data: Record<string, string>) {
  const keys: (readonly unknown[])[] = [];
  switch (event) {
    case "app.updated":
    case "app.removed":
      keys.push(queryKeys.apps(), queryKeys.projects(), queryKeys.services());
      break;
    case "service.updated":
      keys.push(queryKeys.services(), queryKeys.projects(), queryKeys.apps());
      break;
    case "project.updated":
      keys.push(queryKeys.projects(), queryKeys.apps(), queryKeys.services(), queryKeys.audit());
      if (data.slug) keys.push(queryKeys.project(data.slug));
      break;
    case "server.status":
      keys.push(queryKeys.server(), queryKeys.plugins());
      break;
    case "job.updated":
      keys.push(queryKeys.jobs(), queryKeys.audit());
      if (data.app) keys.push(queryKeys.apps(), queryKeys.projects(), queryKeys.app(data.app));
      break;
  }
  for (const queryKey of keys) void qc.invalidateQueries({ queryKey });
}

const events: EventName[] = [
  "app.updated",
  "app.removed",
  "service.updated",
  "project.updated",
  "job.updated",
  "server.status",
];

const maxBackoff = 30_000;

/**
 * Opens one EventSource to /api/v1/events while the shell is mounted and invalidates the
 * matching queries on each event. It reconnects with backoff and reports whether the
 * stream is connected, for the "live updates paused" banner. The stream is not put in
 * the query cache (AGENTS.md).
 */
export function useServerEvents(): { connected: boolean } {
  const qc = useQueryClient();
  const [connected, setConnected] = useState(true);

  useEffect(() => {
    let source: EventSource | undefined;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let backoff = 1000;
    let stopped = false;

    const connect = () => {
      source = new EventSource("/api/v1/events");
      source.onopen = () => {
        // Refetch what may have changed while disconnected.
        if (backoff > 1000) void qc.invalidateQueries();
        backoff = 1000;
        setConnected(true);
      };
      source.onerror = () => {
        source?.close();
        if (stopped) return;
        setConnected(false);
        timer = setTimeout(connect, backoff);
        backoff = Math.min(backoff * 2, maxBackoff);
      };
      for (const name of events) {
        source.addEventListener(name, (e) => {
          let data: Record<string, string> = {};
          try {
            data = JSON.parse((e as MessageEvent<string>).data) as Record<string, string>;
          } catch {
            // An event without a body still invalidates its lists.
          }
          invalidate(qc, name, data);
        });
      }
    };
    connect();
    return () => {
      stopped = true;
      clearTimeout(timer);
      source?.close();
    };
  }, [qc]);

  return { connected };
}
