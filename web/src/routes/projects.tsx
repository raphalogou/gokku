import { useQuery } from "@tanstack/react-query";
import { FolderKanban, Plus } from "lucide-react";
import { useState } from "react";
import { Link, Outlet } from "react-router";
import { ChipList } from "~/components/app/chip";
import { PageHeader } from "~/components/app/page-header";
import { EmptyState, Skeleton } from "~/components/app/states";
import { StatusPill } from "~/components/app/status-pill";
import { buttonVariants } from "~/components/ui/button";
import { SearchInput } from "~/components/ui/search-input";
import { useCan } from "~/hooks/use-can";
import type { AppStatus, ProjectSummary } from "~/lib/api/gen/model";
import { errorMessage } from "~/lib/forms";
import { projectsQuery, serverQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { strings } from "~/lib/strings";
import { cn } from "~/lib/utils";

export function clientLoader() {
  void queryClient.prefetchQuery(projectsQuery());
  void queryClient.prefetchQuery(serverQuery());
  return null;
}

const statusText: Record<AppStatus, string> = {
  running: "text-running-fg",
  stopped: "text-stopped-fg",
  deploying: "text-deploying-fg",
  failed: "text-failed-fg",
};

export default function Projects() {
  const projects = useQuery(projectsQuery());
  const { data: server } = useQuery(serverQuery());
  const canCreate = useCan("project.create");
  const [search, setSearch] = useState("");

  const list = projects.data ?? [];
  const q = search.trim().toLowerCase();
  const shown = q
    ? list.filter((p) => [p.name, p.slug, p.description].some((v) => v.toLowerCase().includes(q)))
    : list;
  const empty = projects.isSuccess && list.length === 0;

  return (
    <>
      <PageHeader
        title={strings.projects.title}
        description={
          projects.isSuccess && !empty
            ? strings.projects.description(list.length, server?.name ?? "")
            : strings.projects.emptyDescription
        }
        actions={
          <>
            {empty ? null : (
              <SearchInput
                label={strings.projects.search}
                value={search}
                onChange={setSearch}
                className="sm:w-[260px]"
              />
            )}
            {canCreate ? (
              <Link to="/projects/new" className={buttonVariants()}>
                <Plus aria-hidden />
                {strings.projects.newProject}
              </Link>
            ) : null}
          </>
        }
      />

      {projects.isPending ? (
        <div
          className="grid grid-cols-[repeat(auto-fill,minmax(min(300px,100%),1fr))] gap-6"
          aria-busy="true"
        >
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-56 rounded-xl" />
          ))}
        </div>
      ) : projects.isError ? (
        <div className="rounded-xl border border-line bg-surface">
          <EmptyState icon={FolderKanban} title={strings.errors.loadFailed}>
            {errorMessage(projects.error)}
          </EmptyState>
        </div>
      ) : empty ? (
        <div className="rounded-xl border border-line bg-surface">
          <EmptyState
            icon={FolderKanban}
            title={strings.projects.emptyTitle}
            action={
              canCreate ? (
                <Link to="/projects/new" className={buttonVariants()}>
                  <Plus aria-hidden />
                  {strings.projects.createProject}
                </Link>
              ) : null
            }
          >
            {strings.projects.emptyBody}
          </EmptyState>
        </div>
      ) : shown.length === 0 ? (
        <p className="py-10 text-center text-muted">{strings.projects.noMatch}</p>
      ) : (
        <ul className="grid grid-cols-[repeat(auto-fill,minmax(min(300px,100%),1fr))] items-start gap-6">
          {shown.map((p) => (
            <li key={p.slug}>
              <ProjectCard project={p} />
            </li>
          ))}
          {canCreate && !q ? (
            <li className="flex min-h-[200px] flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-input-border p-5 text-center">
              <FolderKanban className="size-7 text-muted" aria-hidden />
              <span className="max-w-60 text-muted">{strings.projects.hint}</span>
              <Link to="/projects/new" className={buttonVariants({ variant: "secondary" })}>
                <Plus aria-hidden />
                {strings.projects.createProject}
              </Link>
            </li>
          ) : null}
        </ul>
      )}
      <Outlet />
    </>
  );
}

function ProjectCard({ project: p }: { project: ProjectSummary }) {
  const count = p.health === "failed" ? p.status_counts.failed : p.status_counts.deploying;
  return (
    <section className="flex flex-col gap-3.5 rounded-xl border border-line bg-surface p-5">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2.5">
          <FolderKanban className="size-[22px] shrink-0 text-accent" aria-hidden />
          <h2 className="min-w-0 truncate text-lg font-bold">
            <Link to={`/projects/${p.slug}`} className="hover:underline">
              {p.name}
            </Link>
          </h2>
        </div>
        <StatusPill kind={p.health} label={strings.projects.health(p.health, count)} />
      </div>
      {p.description ? <p className="text-muted">{p.description}</p> : null}
      <div className="border-t border-line pt-2">
        {p.apps.length === 0 ? (
          <p className="py-1.5 text-sm text-muted">{strings.projects.noApps}</p>
        ) : (
          <ul>
            {p.apps.map((a) => (
              <li key={a.name} className="flex min-h-8 items-center justify-between gap-2">
                <Link to={`/apps/${a.name}`} className="truncate font-medium hover:underline">
                  {a.name}
                </Link>
                <span className={cn("text-[13px] font-semibold", statusText[a.status])}>
                  {strings.appStatus[a.status]}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
      <div className="flex flex-col gap-2.5 border-t border-line pt-3.5">
        <span className="text-[13px] font-semibold text-muted">
          {strings.projects.counts(p.apps.length, p.services.length)}
        </span>
        <ChipList items={p.services.map((s) => `${s.name} ${s.type}`)} empty={strings.projects.noServices} />
      </div>
    </section>
  );
}
