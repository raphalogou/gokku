import { useQuery } from "@tanstack/react-query";
import { Database, FolderKanban, Info, Plus } from "lucide-react";
import { useState } from "react";
import { Link, Outlet } from "react-router";
import { Card, CardHeader } from "~/components/app/card";
import { ChipList } from "~/components/app/chip";
import { DataTable, TD, TH, THead, TR } from "~/components/app/data-table";
import { DatastoreIcon, DatastoreType } from "~/components/app/datastore-icon";
import { PageHeader } from "~/components/app/page-header";
import { EmptyState, Skeleton } from "~/components/app/states";
import { StatusPill } from "~/components/app/status-pill";
import { buttonVariants } from "~/components/ui/button";
import { SearchInput } from "~/components/ui/search-input";
import { useCan } from "~/hooks/use-can";
import type { ServiceSummary } from "~/lib/api/gen/model";
import { errorMessage } from "~/lib/forms";
import { pluginsQuery, serverQuery, servicesQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { datastoreName, serviceStatusKind } from "~/lib/status";
import { strings } from "~/lib/strings";
import { cn } from "~/lib/utils";

export function clientLoader() {
  void queryClient.prefetchQuery(servicesQuery());
  void queryClient.prefetchQuery(pluginsQuery());
  return null;
}

/** Services list (SVC-2): filter by type, search, create for admins, and a note about
 * datastore plugins that are not installed. */
export default function Services() {
  const services = useQuery(servicesQuery());
  const { data: server } = useQuery(serverQuery());
  const { data: plugins } = useQuery(pluginsQuery());
  const [type, setType] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const canCreate = useCan("service.create");
  const t = strings.services;

  const list = services.data ?? [];
  const types = [...new Set(list.map((s) => s.type))];
  const q = search.trim().toLowerCase();
  const shown = list.filter((s) => (!type || s.type === type) && (!q || s.name.toLowerCase().includes(q)));
  const missing = (plugins?.available ?? [])
    .filter((p) => p.kind === "datastore")
    .map((p) => datastoreName(p.name));
  const noPlugins = !!server?.reachable && server.datastores.length === 0;

  const chip = (value: string | null, label: string, count: number, icon?: string) => (
    <button
      key={value ?? "all"}
      type="button"
      aria-pressed={type === value}
      onClick={() => setType(value)}
      className={cn(
        "inline-flex min-h-11 cursor-pointer items-center gap-2 rounded-full border px-4 text-sm font-semibold",
        type === value
          ? "border-accent bg-nav-active text-accent"
          : "border-input-border bg-surface text-ink",
      )}
    >
      {icon ? <DatastoreIcon type={icon} /> : null}
      {label} {count}
    </button>
  );

  return (
    <>
      <PageHeader
        title={t.title}
        description={t.description(server?.name ?? "")}
        actions={
          <>
            <SearchInput label={t.search} value={search} onChange={setSearch} />
            {canCreate && (server?.datastores.length ?? 0) > 0 ? (
              <Link to="/services/new" className={buttonVariants()}>
                <Plus aria-hidden />
                {t.create}
              </Link>
            ) : null}
          </>
        }
      />

      {types.length > 1 ? (
        <fieldset className="mb-4 flex flex-wrap gap-2">
          <legend className="sr-only">{t.filterType}</legend>
          {chip(null, t.all, list.length)}
          {types.map((ty) => chip(ty, datastoreName(ty), list.filter((s) => s.type === ty).length, ty))}
        </fieldset>
      ) : null}

      <Card>
        <CardHeader title={t.allServices} />
        {services.isPending ? (
          <div className="flex flex-col gap-3 p-5" aria-busy="true">
            {[0, 1, 2].map((i) => (
              <Skeleton key={i} className="h-10" />
            ))}
          </div>
        ) : services.isError ? (
          <EmptyState icon={Database} title={strings.errors.loadFailed}>
            {errorMessage(services.error)}
          </EmptyState>
        ) : noPlugins ? (
          <EmptyState
            icon={Database}
            title={t.noPluginsTitle}
            action={
              <Link to="/plugins" className="font-semibold text-accent hover:underline">
                {strings.nav.plugins}
              </Link>
            }
          >
            {t.noPluginsBody}
          </EmptyState>
        ) : list.length === 0 ? (
          <EmptyState icon={Database} title={t.emptyTitle}>
            {t.emptyBody}
          </EmptyState>
        ) : shown.length === 0 ? (
          <p className="px-5 py-10 text-center text-muted">{t.noMatch}</p>
        ) : (
          <ServicesTable services={shown} />
        )}
      </Card>

      {missing.length > 0 && !noPlugins ? (
        <div className="mt-6 flex items-start gap-3 rounded-xl border border-line bg-surface px-4 py-3.5">
          <Info className="mt-0.5 size-[18px] shrink-0 text-accent" aria-hidden />
          <span>
            {t.notInstalled(missing)}{" "}
            <Link to="/plugins" className="font-semibold text-accent hover:underline">
              {t.pluginsLink}
            </Link>{" "}
            {t.toAdd}
          </span>
        </div>
      ) : null}
      <Outlet />
    </>
  );
}

const serviceHref = (s: ServiceSummary) => `/services/${s.type}/${s.name}`;

function ServiceProject({ s }: { s: ServiceSummary }) {
  return s.project ? (
    <Link to={`/projects/${s.project.slug}`} className="inline-flex items-center gap-1.5 hover:underline">
      <FolderKanban className="size-3.5 text-muted" aria-hidden />
      {s.project.name}
    </Link>
  ) : (
    <span className="text-muted">{strings.apps.unassigned}</span>
  );
}

function ServicesTable({ services }: { services: ServiceSummary[] }) {
  const t = strings.services;
  return (
    <>
      {/* Below 640 px the table becomes stacked cards (DESIGN.md, 7). */}
      <ul className="divide-y divide-line sm:hidden">
        {services.map((s) => (
          <li key={`${s.type}/${s.name}`} className="flex flex-col gap-1.5 px-5 py-4">
            <div className="flex items-center justify-between gap-3">
              <Link to={serviceHref(s)} className="truncate text-[17px] font-bold hover:underline">
                {s.name}
              </Link>
              <StatusPill kind={serviceStatusKind[s.status]} label={strings.serviceStatus[s.status]} />
            </div>
            <div className="flex flex-wrap gap-x-3 gap-y-1 text-sm text-muted">
              <DatastoreType type={s.type} />
              <ServiceProject s={s} />
            </div>
            <ChipList items={s.links} empty={t.none} />
          </li>
        ))}
      </ul>
      <div className="hidden sm:block">
        <DataTable label={t.allServices} className="min-w-[760px]">
          <THead>
            <TH>{t.columns.name}</TH>
            <TH>{t.columns.type}</TH>
            <TH>{t.columns.status}</TH>
            <TH>{t.columns.project}</TH>
            <TH>{t.columns.links}</TH>
          </THead>
          <tbody>
            {services.map((s) => (
              <TR key={`${s.type}/${s.name}`}>
                <TD>
                  <Link to={serviceHref(s)} className="font-semibold hover:underline">
                    {s.name}
                  </Link>
                  <span className="block font-mono text-[12px] text-muted">{s.version}</span>
                </TD>
                <TD>
                  <DatastoreType type={s.type} />
                </TD>
                <TD>
                  <StatusPill kind={serviceStatusKind[s.status]} label={strings.serviceStatus[s.status]} />
                </TD>
                <TD>
                  <ServiceProject s={s} />
                </TD>
                <TD>
                  <ChipList items={s.links} empty={t.none} />
                </TD>
              </TR>
            ))}
          </tbody>
        </DataTable>
      </div>
    </>
  );
}
