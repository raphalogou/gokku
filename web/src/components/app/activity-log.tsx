import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Activity, Download } from "lucide-react";
import { useDeferredValue, useState } from "react";
import { Link } from "react-router";
import { useCan } from "~/hooks/use-can";
import { getExportAuditUrl } from "~/lib/api/gen/client";
import type { ListAuditParams, ListAuditResult, ListAuditSource } from "~/lib/api/gen/model";
import { dateTime, timeAgo } from "~/lib/format";
import { errorMessage } from "~/lib/forms";
import { auditQuery, projectsQuery } from "~/lib/queries";
import { strings } from "~/lib/strings";
import { Button, buttonVariants } from "../ui/button";
import { SearchInput } from "../ui/search-input";
import { Select } from "../ui/select";
import { Card, CardHeader } from "./card";
import { DataTable, TD, TH, THead, TR } from "./data-table";
import { EmptyState, Skeleton } from "./states";
import { StatusPill } from "./status-pill";

const any = "*";

/** The audit log with filters and cursor pages (ACT-2). With a project, it shows only that
 * project's entries and drops the project filter. */
export function ActivityLog({ project, actions }: { project?: string; actions?: "header" | "inline" }) {
  const t = strings.activity;
  const [search, setSearch] = useState("");
  const [user, setUser] = useState(any);
  const [projectFilter, setProjectFilter] = useState(any);
  const [result, setResult] = useState(any);
  const [source, setSource] = useState(any);
  const q = useDeferredValue(search.trim());
  const canExport = useCan("audit.export");
  const projects = useQuery({ ...projectsQuery(), enabled: !project });

  const params: Omit<ListAuditParams, "cursor"> = {};
  if (q) params.q = q;
  if (user !== any) params.user = user;
  const slug = project ?? (projectFilter !== any ? projectFilter : undefined);
  if (slug) params.project = slug;
  if (result !== any) params.result = result as ListAuditResult;
  if (source !== any) params.source = source as ListAuditSource;
  const audit = useInfiniteQuery(auditQuery(params));

  const items = audit.data?.pages.flatMap((p) => p.items) ?? [];
  const actors = audit.data?.pages[0]?.actors ?? [];
  const projectNames = new Map((projects.data ?? []).map((p) => [p.slug, p.name]));

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <SearchInput label={t.search} value={search} onChange={setSearch} />
        <Select
          aria-label={t.user}
          value={user}
          onValueChange={setUser}
          options={[{ value: any, label: t.allUsers }, ...actors.map((a) => ({ value: a, label: a }))]}
          className="w-full sm:w-[170px]"
        />
        {project ? null : (
          <Select
            aria-label={t.project}
            value={projectFilter}
            onValueChange={setProjectFilter}
            options={[
              { value: any, label: t.allProjects },
              ...(projects.data ?? []).map((p) => ({ value: p.slug, label: p.name })),
            ]}
            className="w-full sm:w-[190px]"
          />
        )}
        <Select
          aria-label={t.result}
          value={result}
          onValueChange={setResult}
          options={[
            { value: any, label: t.anyResult },
            { value: "ok", label: t.succeeded },
            { value: "fail", label: t.failed },
          ]}
          className="w-full sm:w-[160px]"
        />
        <Select
          aria-label={t.source}
          value={source}
          onValueChange={setSource}
          options={[
            { value: any, label: t.anySource },
            ...(["web", "api", "git", "system"] as const).map((s) => ({ value: s, label: t.sources[s] })),
          ]}
          className="w-full sm:w-[160px]"
        />
        {canExport && actions === "inline" ? <ExportLink params={params} /> : null}
      </div>

      <Card>
        <CardHeader
          title={t.events}
          actions={canExport && actions === "header" ? <ExportLink params={params} /> : null}
        />
        {audit.isPending ? (
          <div className="flex flex-col gap-3 p-5" aria-busy="true">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-10" />
            ))}
          </div>
        ) : audit.isError ? (
          <EmptyState icon={Activity} title={strings.errors.loadFailed}>
            {errorMessage(audit.error)}
          </EmptyState>
        ) : items.length === 0 ? (
          <p className="px-5 py-10 text-center text-muted">{project && !q ? t.emptyProject : t.empty}</p>
        ) : (
          <DataTable label={t.events} className="min-w-[880px]">
            <THead>
              <TH>{t.columns.time}</TH>
              <TH>{t.columns.user}</TH>
              <TH>{t.columns.action}</TH>
              <TH>{t.columns.target}</TH>
              {project ? null : <TH>{t.columns.project}</TH>}
              <TH>{t.columns.result}</TH>
              <TH>{t.columns.source}</TH>
            </THead>
            <tbody>
              {items.map((e) => (
                <TR key={e.id}>
                  <TD className="whitespace-nowrap text-muted">
                    <time dateTime={e.ts} title={dateTime(e.ts)}>
                      {timeAgo(e.ts)}
                    </time>
                  </TD>
                  <TD className="font-semibold">{e.actor_name}</TD>
                  <TD>
                    {e.job_id ? (
                      <Link
                        to={`/jobs/${e.job_id}`}
                        className="font-mono text-[13px] hover:underline"
                        title={t.viewJob}
                      >
                        {e.action}
                      </Link>
                    ) : (
                      <span className="font-mono text-[13px]">{e.action}</span>
                    )}
                  </TD>
                  <TD className="break-all">{e.target_name || "—"}</TD>
                  {project ? null : (
                    <TD className={e.project_slug ? "" : "text-muted"}>
                      {e.project_slug ? (projectNames.get(e.project_slug) ?? e.project_slug) : t.none}
                    </TD>
                  )}
                  <TD>
                    <StatusPill
                      kind={e.result === "ok" ? "running" : "failed"}
                      label={e.result === "ok" ? t.succeeded : t.failed}
                    />
                  </TD>
                  <TD className="whitespace-nowrap">{t.sources[e.source]}</TD>
                </TR>
              ))}
            </tbody>
          </DataTable>
        )}
        {items.length > 0 ? (
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line px-5 py-3">
            <span className="text-sm text-muted">{t.showing(items.length)}</span>
            {audit.hasNextPage ? (
              <Button
                variant="secondary"
                onClick={() => void audit.fetchNextPage()}
                disabled={audit.isFetchingNextPage}
              >
                {audit.isFetchingNextPage ? t.loading : t.loadMore}
              </Button>
            ) : null}
          </div>
        ) : null}
      </Card>
    </div>
  );
}

function ExportLink({ params }: { params: Omit<ListAuditParams, "cursor"> }) {
  return (
    <a href={getExportAuditUrl(params)} download className={buttonVariants({ variant: "secondary" })}>
      <Download aria-hidden />
      {strings.activity.export}
    </a>
  );
}
