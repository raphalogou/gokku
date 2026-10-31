import { useSuspenseQuery } from "@tanstack/react-query";
import { FolderX } from "lucide-react";
import { useState } from "react";
import { isRouteErrorResponse, Link, Outlet, useRouteError } from "react-router";
import { Breadcrumbs } from "~/components/app/breadcrumbs";
import { ProjectActions } from "~/components/app/project-actions";
import { AddMemberDialog } from "~/components/app/project-members";
import { RouteTabs } from "~/components/app/route-tabs";
import { EmptyState } from "~/components/app/states";
import { StatusPill } from "~/components/app/status-pill";
import { Button, buttonVariants } from "~/components/ui/button";
import { useCan } from "~/hooks/use-can";
import { ApiError } from "~/lib/api/fetcher";
import { projectQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/project";

export async function clientLoader({ params }: Route.ClientLoaderArgs) {
  await queryClient.ensureQueryData(projectQuery(params.slug));
  return null;
}

/** The project header and tabs around each project screen (DESIGN.md, 7). */
export default function Project({ params }: Route.ComponentProps) {
  const { data: p } = useSuspenseQuery(projectQuery(params.slug));
  const canEdit = useCan("project.members.write");
  const [adding, setAdding] = useState<"app" | "service" | null>(null);
  const count = p.health === "failed" ? p.status_counts.failed : p.status_counts.deploying;
  const base = `/projects/${p.slug}`;

  return (
    <>
      <Breadcrumbs items={[{ to: "/projects", label: strings.project.breadcrumb }, { label: p.name }]} />
      <header className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="text-[28px] leading-tight font-bold">{p.name}</h1>
            <StatusPill kind={p.health} label={strings.projects.health(p.health, count)} />
          </div>
          {p.description ? <p className="mt-1 text-[15px] text-muted">{p.description}</p> : null}
        </div>
        <div className="flex flex-wrap gap-3">
          <ProjectActions project={p} />
          {canEdit ? (
            <>
              <Button variant="secondary" onClick={() => setAdding("service")}>
                {strings.project.addService}
              </Button>
              <Button onClick={() => setAdding("app")}>{strings.project.addApp}</Button>
            </>
          ) : null}
        </div>
      </header>
      <RouteTabs
        label={p.name}
        tabs={[
          { to: base, label: strings.project.tabs.overview, end: true },
          { to: `${base}/apps`, label: strings.project.tabs.apps },
          { to: `${base}/services`, label: strings.project.tabs.services },
          { to: `${base}/config`, label: strings.project.tabs.config },
          { to: `${base}/activity`, label: strings.project.tabs.activity },
          { to: `${base}/settings`, label: strings.project.tabs.settings },
        ]}
      />
      <Outlet context={{ openAdd: canEdit ? setAdding : undefined }} />
      {adding ? <AddMemberDialog project={p} kind={adding} onClose={() => setAdding(null)} /> : null}
    </>
  );
}

export function ErrorBoundary() {
  const error = useRouteError();
  const notFound =
    (error instanceof ApiError && error.status === 404) ||
    (isRouteErrorResponse(error) && error.status === 404);
  return (
    <EmptyState
      icon={FolderX}
      title={notFound ? strings.errors.notFoundTitle : strings.errors.loadFailed}
      action={
        <Link to="/projects" className={buttonVariants({ variant: "secondary" })}>
          {strings.errors.goHome}
        </Link>
      }
    >
      {error instanceof ApiError ? error.message : strings.errors.unexpected}
    </EmptyState>
  );
}
