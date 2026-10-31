import { useQuery } from "@tanstack/react-query";
import { Boxes, ExternalLink, FolderKanban, Plus } from "lucide-react";
import { useState } from "react";
import { Link, Outlet } from "react-router";
import { Card, CardHeader } from "~/components/app/card";
import { ChipList } from "~/components/app/chip";
import { DataTable, TD, TH, THead, TR } from "~/components/app/data-table";
import { LastDeploy } from "~/components/app/last-deploy";
import { PageHeader } from "~/components/app/page-header";
import { EmptyState, Skeleton } from "~/components/app/states";
import { StatusPill } from "~/components/app/status-pill";
import { buttonVariants } from "~/components/ui/button";
import { SearchInput } from "~/components/ui/search-input";
import { Select } from "~/components/ui/select";
import { useCan } from "~/hooks/use-can";
import type { AppStatus, AppSummary } from "~/lib/api/gen/model";
import { errorMessage } from "~/lib/forms";
import { appsQuery, projectsQuery, serverQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { strings } from "~/lib/strings";

export function clientLoader() {
  void queryClient.prefetchQuery(appsQuery());
  void queryClient.prefetchQuery(projectsQuery());
  return null;
}

const statuses: AppStatus[] = ["running", "deploying", "stopped", "failed"];
const allProjects = "*";
const unassigned = "-";

/** Apps list (APP-1): status, project, domain, last deploy and services, with search and a
 * project filter. */
export default function Apps() {
  const apps = useQuery(appsQuery());
  const projects = useQuery(projectsQuery());
  const { data: server } = useQuery(serverQuery());
  const [search, setSearch] = useState("");
  const [project, setProject] = useState(allProjects);
  const canCreate = useCan("app.create");
  const t = strings.apps;
  const serverName = server?.name ?? "";

  const list = apps.data ?? [];
  const q = search.trim().toLowerCase();
  const shown = list.filter(
    (a) =>
      (project === allProjects || (project === unassigned ? !a.project : a.project?.slug === project)) &&
      (!q || a.name.includes(q) || a.domains.some((d) => d.includes(q))),
  );
  const projectOptions = [
    { value: allProjects, label: t.allProjects },
    ...(projects.data ?? []).map((p) => ({ value: p.slug, label: p.name })),
    { value: unassigned, label: t.unassigned },
  ];

  return (
    <>
      <PageHeader
        title={t.title}
        description={apps.isPending ? t.loading(serverName) : t.description(list.length, serverName)}
        actions={
          <>
            <SearchInput label={t.search} value={search} onChange={setSearch} />
            <Select
              aria-label={t.filterProject}
              value={project}
              onValueChange={setProject}
              options={projectOptions}
              className="w-full sm:w-[200px]"
            />
            {canCreate ? (
              <Link to="/apps/new" className={buttonVariants()}>
                <Plus aria-hidden />
                {strings.newApp.title}
              </Link>
            ) : null}
          </>
        }
      />

      {apps.isSuccess && list.length > 0 ? (
        <dl className="mb-6 grid grid-cols-2 gap-4 md:grid-cols-4">
          {statuses.map((s) => (
            <div key={s} className="rounded-xl border border-line bg-surface px-5 py-4">
              <dt>
                <StatusPill kind={s} label={strings.appStatus[s]} />
              </dt>
              <dd className="mt-2 text-2xl font-bold">{list.filter((a) => a.status === s).length}</dd>
            </div>
          ))}
        </dl>
      ) : null}

      <Card>
        <CardHeader title={t.allApps} />
        {apps.isPending ? (
          <div className="flex flex-col gap-3 p-5" aria-busy="true">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-10" />
            ))}
          </div>
        ) : apps.isError ? (
          <EmptyState icon={Boxes} title={strings.errors.loadFailed}>
            {errorMessage(apps.error)}
          </EmptyState>
        ) : list.length === 0 ? (
          <EmptyState icon={Boxes} title={t.emptyTitle}>
            {t.emptyBody}
          </EmptyState>
        ) : shown.length === 0 ? (
          <p className="px-5 py-10 text-center text-muted">{t.noMatch}</p>
        ) : (
          <AppsTable apps={shown} />
        )}
      </Card>
      <Outlet />
    </>
  );
}

function AppsTable({ apps }: { apps: AppSummary[] }) {
  const t = strings.apps;
  return (
    <>
      {/* Below 640 px the table becomes stacked cards (DESIGN.md, 7). */}
      <ul className="divide-y divide-line sm:hidden">
        {apps.map((a) => (
          <li key={a.name} className="flex flex-col gap-1.5 px-5 py-4">
            <div className="flex items-center justify-between gap-3">
              <Link to={`/apps/${a.name}`} className="truncate text-[17px] font-bold hover:underline">
                {a.name}
              </Link>
              <StatusPill kind={a.status} label={strings.appStatus[a.status]} />
            </div>
            <div className="flex flex-wrap gap-x-3 gap-y-1 text-sm text-muted">
              <span>{a.project?.name ?? t.unassigned}</span>
              <span>{a.domains[0] ?? t.noDomain}</span>
            </div>
            <LastDeploy app={a} />
          </li>
        ))}
      </ul>
      <div className="hidden sm:block">
        <DataTable label={t.allApps} className="min-w-[900px]">
          <THead>
            <TH>{t.columns.name}</TH>
            <TH>{t.columns.status}</TH>
            <TH>{t.columns.project}</TH>
            <TH>{t.columns.domain}</TH>
            <TH>{t.columns.lastDeploy}</TH>
            <TH>{t.columns.services}</TH>
          </THead>
          <tbody>
            {apps.map((a) => (
              <TR key={a.name}>
                <TD>
                  <Link to={`/apps/${a.name}`} className="font-semibold hover:underline">
                    {a.name}
                  </Link>
                </TD>
                <TD>
                  <StatusPill kind={a.status} label={strings.appStatus[a.status]} />
                </TD>
                <TD>
                  {a.project ? (
                    <Link
                      to={`/projects/${a.project.slug}`}
                      className="inline-flex items-center gap-1.5 hover:underline"
                    >
                      <FolderKanban className="size-3.5 text-muted" aria-hidden />
                      {a.project.name}
                    </Link>
                  ) : (
                    <span className="text-muted">{t.unassigned}</span>
                  )}
                </TD>
                <TD>
                  {a.domains[0] ? (
                    <a
                      href={`http://${a.domains[0]}`}
                      target="_blank"
                      rel="noreferrer"
                      className="inline-flex items-center gap-1 hover:underline"
                    >
                      {a.domains[0]}
                      <ExternalLink className="size-3.5 text-muted" aria-hidden />
                    </a>
                  ) : (
                    <span className="text-muted">{t.noDomain}</span>
                  )}
                </TD>
                <TD>
                  <LastDeploy app={a} />
                </TD>
                <TD>
                  <ChipList items={a.services.map((s) => s.type)} empty={t.none} />
                </TD>
              </TR>
            ))}
          </tbody>
        </DataTable>
      </div>
    </>
  );
}
