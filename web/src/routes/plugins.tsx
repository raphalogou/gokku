import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Puzzle, RefreshCw } from "lucide-react";
import { Card, CardHeader } from "~/components/app/card";
import { CodeBlock } from "~/components/app/code-block";
import { DataTable, TD, TH, THead, TR } from "~/components/app/data-table";
import { PageHeader } from "~/components/app/page-header";
import { EmptyState, Skeleton } from "~/components/app/states";
import { Button } from "~/components/ui/button";
import { notify } from "~/components/ui/toast";
import { useCan } from "~/hooks/use-can";
import { rescanServer } from "~/lib/api/gen/client";
import type { PluginKind } from "~/lib/api/gen/model";
import { errorMessage } from "~/lib/forms";
import { pluginsQuery, queryKeys, serverQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { datastoreName } from "~/lib/status";
import { strings } from "~/lib/strings";

export function clientLoader() {
  void queryClient.prefetchQuery(pluginsQuery());
  return null;
}

function pluginText(name: string, kind: PluginKind) {
  const t = strings.plugins;
  switch (kind) {
    case "datastore":
      return { provides: t.datastoreProvides(datastoreName(name)), unlocks: t.datastoreUnlocks };
    case "letsencrypt":
      return { provides: t.letsencryptProvides, unlocks: t.letsencryptUnlocks };
    default:
      return { provides: t.otherProvides, unlocks: t.otherUnlocks };
  }
}

/** Plugins (PLG-1): what is installed and what it unlocks, recognised plugins that are
 * not installed with the install command, and Rescan for admins (PLG-2). */
export default function Plugins() {
  const plugins = useQuery(pluginsQuery());
  const { data: server } = useQuery(serverQuery());
  const canRescan = useCan("server.rescan");
  const qc = useQueryClient();
  const t = strings.plugins;

  const rescan = useMutation({
    mutationFn: () => rescanServer(),
    onSuccess: async (info) => {
      qc.setQueryData(queryKeys.server(), info);
      await qc.invalidateQueries({ queryKey: queryKeys.plugins() });
      notify(
        info.reachable ? t.rescanned : (info.error ?? strings.server.unreachableTitle(info.name)),
        info.reachable ? "success" : "error",
      );
    },
    onError: (err) => notify(errorMessage(err), "error"),
  });

  const probed = server ? server.dokku_version != null : true;
  const columns = (unlockLabel: string) => (
    <THead>
      <TH>{t.columns.plugin}</TH>
      <TH>{t.columns.provides}</TH>
      <TH>{unlockLabel}</TH>
      <TH className="w-40">{t.columns.status}</TH>
    </THead>
  );

  return (
    <>
      <PageHeader
        title={t.title}
        description={t.description(server?.name ?? "")}
        actions={
          canRescan ? (
            <Button variant="secondary" onClick={() => rescan.mutate()} disabled={rescan.isPending}>
              <RefreshCw aria-hidden className={rescan.isPending ? "animate-spin" : undefined} />
              {rescan.isPending ? t.rescanning : t.rescan}
            </Button>
          ) : null
        }
      />
      {plugins.isPending ? (
        <Skeleton className="h-64 rounded-xl" />
      ) : plugins.isError ? (
        <Card>
          <EmptyState icon={Puzzle} title={strings.errors.loadFailed}>
            {errorMessage(plugins.error)}
          </EmptyState>
        </Card>
      ) : !probed ? (
        <Card>
          <EmptyState icon={Puzzle} title={t.probeFailedTitle}>
            {t.probeFailedBody}
          </EmptyState>
        </Card>
      ) : (
        <div className="flex flex-col gap-6">
          <Card>
            <CardHeader title={t.installed} />
            {plugins.data.installed.length === 0 ? (
              <p className="px-5 py-6 text-muted">{t.noneInstalled}</p>
            ) : (
              <DataTable label={t.installed} className="min-w-[760px]">
                {columns(t.columns.unlocks)}
                <tbody>
                  {plugins.data.installed.map((p) => {
                    const d = pluginText(p.name, p.kind);
                    return (
                      <TR key={p.name}>
                        <TD>
                          <span className="font-mono font-medium">{p.name}</span>
                          <span className="block font-mono text-[12px] text-muted">{p.version}</span>
                        </TD>
                        <TD>{d.provides}</TD>
                        <TD className={p.reason ? "text-muted" : undefined}>{p.reason ?? d.unlocks}</TD>
                        <TD>
                          {p.usable ? (
                            <span className="inline-flex items-center gap-1.5 text-sm font-semibold text-running-fg">
                              <Check className="size-4" aria-hidden />
                              {t.statusInstalled}
                            </span>
                          ) : (
                            <span className="text-sm font-semibold text-muted">
                              {p.kind === "other" ? t.statusInstalled : t.statusUnusable}
                            </span>
                          )}
                        </TD>
                      </TR>
                    );
                  })}
                </tbody>
              </DataTable>
            )}
          </Card>

          <Card>
            <CardHeader title={t.available} />
            {plugins.data.available.length === 0 ? (
              <p className="px-5 py-6 text-muted">{t.allInstalled}</p>
            ) : (
              <>
                <DataTable label={t.available} className="min-w-[760px]">
                  {columns(t.columns.wouldUnlock)}
                  <tbody>
                    {plugins.data.available.map((p) => {
                      const d = pluginText(p.name, p.kind);
                      return (
                        <TR key={p.name}>
                          <TD className="font-mono font-medium">{p.name}</TD>
                          <TD>{d.provides}</TD>
                          <TD>{d.unlocks}</TD>
                          <TD className="text-sm font-semibold text-muted">{t.statusNotInstalled}</TD>
                        </TR>
                      );
                    })}
                  </tbody>
                </DataTable>
                <div className="flex flex-col gap-2.5 border-t border-line px-5 py-4">
                  <span className="text-sm text-muted">{t.installHint}</span>
                  <CodeBlock value="sudo dokku plugin:install <git-url> <name>" label={t.installCommand} />
                  <ul className="flex flex-col gap-1 text-[13px] text-muted">
                    {plugins.data.available.map((p) => (
                      <li key={p.name} className="font-mono break-all">
                        sudo dokku plugin:install {p.git_url} {p.name}
                      </li>
                    ))}
                  </ul>
                </div>
              </>
            )}
          </Card>
        </div>
      )}
    </>
  );
}
