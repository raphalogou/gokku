import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, ServerCrash, WifiOff } from "lucide-react";
import { useCan } from "~/hooks/use-can";
import { rescanServer } from "~/lib/api/gen/client";
import type { ServerInfo } from "~/lib/api/gen/model";
import { timeAgo } from "~/lib/format";
import { queryKeys, serverQuery } from "~/lib/queries";
import { strings } from "~/lib/strings";
import { cn } from "~/lib/utils";
import { Button } from "../ui/button";

/** The server chip in the sidebar: name, plus a dot and a word for the connection. */
export function ServerChip() {
  const { data: server } = useQuery(serverQuery());
  const state = !server ? "checking" : server.reachable ? "connected" : "unreachable";
  return (
    <div className="flex items-center gap-2.5 rounded-[10px] border border-line px-3 py-3">
      <span
        className={cn(
          "size-2 shrink-0 rounded-full",
          state === "connected" ? "bg-running-fg" : state === "unreachable" ? "bg-failed-fg" : "bg-muted",
        )}
        aria-hidden
      />
      <div className="flex min-w-0 flex-col">
        <span className="truncate font-mono text-[13px] font-medium">
          {server?.name ?? strings.nav.server}
        </span>
        <span className="text-[13px] text-muted">{strings.server[state]}</span>
      </div>
    </div>
  );
}

/**
 * Banners above every signed-in page: "Cannot reach Dokku" with diagnostics while the
 * server is unreachable (SRV-2), and a quiet note while live updates are disconnected.
 */
export function ServerProblems({ eventsConnected }: { eventsConnected: boolean }) {
  const { data: server } = useQuery(serverQuery());
  return (
    <>
      {server && !server.reachable ? <DokkuUnreachable server={server} /> : null}
      {eventsConnected ? null : (
        <div
          role="status"
          className="mb-4 flex items-center gap-2 rounded-lg border border-line bg-subtle px-4 py-2 text-sm text-muted"
        >
          <WifiOff className="size-4 shrink-0" aria-hidden />
          {strings.server.liveUpdatesPaused}
        </div>
      )}
    </>
  );
}

function DokkuUnreachable({ server }: { server: ServerInfo }) {
  const qc = useQueryClient();
  const canRescan = useCan("server.rescan");
  // Admins re-run the probe; everyone else asks for the latest status, which the
  // background refresh keeps trying.
  const retry = useMutation({
    mutationFn: async () => (canRescan ? rescanServer() : qc.fetchQuery({ ...serverQuery(), staleTime: 0 })),
    onSuccess: (info) => {
      qc.setQueryData(queryKeys.server(), info);
      if (info.reachable) void qc.invalidateQueries();
    },
  });

  return (
    <section
      aria-labelledby="dokku-unreachable"
      className="mb-6 rounded-xl border border-failed bg-surface"
      role="alert"
    >
      <div className="flex flex-wrap items-start gap-4 p-5">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-failed-bg text-failed-fg">
          <ServerCrash className="size-5" aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <h2 id="dokku-unreachable" className="text-base font-bold">
            {strings.server.unreachableTitle(server.name)}
          </h2>
          <p className="mt-1 text-[15px] text-muted">
            {strings.server.unreachableBody(server.last_ok_at ? timeAgo(server.last_ok_at) : null)}
          </p>
        </div>
        <Button variant="secondary" onClick={() => retry.mutate()} disabled={retry.isPending}>
          <RefreshCw aria-hidden className={retry.isPending ? "animate-spin" : undefined} />
          {retry.isPending ? strings.server.retrying : strings.server.retry}
        </Button>
      </div>
      <div className="grid gap-5 border-t border-line p-5 md:grid-cols-2">
        <div className="min-w-0">
          <h3 className="mb-2 text-sm font-semibold text-muted">{strings.server.diagnostics}</h3>
          <pre className="overflow-x-auto rounded-lg bg-code-bg px-4 py-3 font-mono text-[13px] leading-relaxed whitespace-pre-wrap">
            {server.error_command ? `$ ${server.error_command}\n` : null}
            {server.error}
          </pre>
        </div>
        <div className="min-w-0">
          <h3 className="mb-2 text-sm font-semibold text-muted">{strings.server.whatToCheck}</h3>
          <ul className="list-disc space-y-1.5 pl-5 text-[15px]">
            {strings.server.checks.map((c) => (
              <li key={c}>{c}</li>
            ))}
          </ul>
        </div>
      </div>
    </section>
  );
}
