import { useSuspenseQuery } from "@tanstack/react-query";
import { BoxSelect } from "lucide-react";
import { isRouteErrorResponse, Link, Outlet, useRouteError } from "react-router";
import { Breadcrumbs, type Crumb } from "~/components/app/breadcrumbs";
import { RouteTabs } from "~/components/app/route-tabs";
import { EmptyState } from "~/components/app/states";
import { StatusPill } from "~/components/app/status-pill";
import { buttonVariants } from "~/components/ui/button";
import { ApiError } from "~/lib/api/fetcher";
import { appQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/app";

export async function clientLoader({ params }: Route.ClientLoaderArgs) {
  await queryClient.ensureQueryData(appQuery(params.name));
  return null;
}

/** The app header and tabs around each app screen. Actions arrive with jobs in M3. */
export default function App({ params }: Route.ComponentProps) {
  const { data: app } = useSuspenseQuery(appQuery(params.name));
  const base = `/apps/${app.name}`;
  const t = strings.app.tabs;
  const crumbs: Crumb[] = app.project
    ? [
        { to: "/projects", label: strings.project.breadcrumb },
        { to: `/projects/${app.project.slug}`, label: app.project.name },
        { label: app.name },
      ]
    : [{ to: "/apps", label: strings.apps.title }, { label: app.name }];

  return (
    <>
      <Breadcrumbs items={crumbs} />
      <header className="mb-6 min-w-0">
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="font-mono text-[28px] leading-tight font-bold break-all">{app.name}</h1>
          <StatusPill kind={app.status} label={strings.appStatus[app.status]} />
        </div>
        <p className="mt-1 flex flex-wrap gap-x-3 text-[15px] text-muted">
          <span>{app.project?.name ?? strings.apps.unassigned}</span>
          <span>{app.domains[0] ?? strings.apps.noDomain}</span>
        </p>
      </header>
      <RouteTabs
        label={app.name}
        tabs={[
          { to: base, label: t.overview, end: true },
          { to: `${base}/config`, label: t.config },
          { to: `${base}/domains`, label: t.domains },
          { to: `${base}/logs`, label: t.logs },
          { to: `${base}/scaling`, label: t.scaling },
          { to: `${base}/deploy`, label: t.deploy },
          { to: `${base}/settings`, label: t.settings },
        ]}
      />
      <Outlet />
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
      icon={BoxSelect}
      title={notFound ? strings.app.notFoundTitle : strings.errors.loadFailed}
      action={
        <Link to="/apps" className={buttonVariants({ variant: "secondary" })}>
          {strings.apps.title}
        </Link>
      }
    >
      {notFound
        ? strings.app.notFoundBody
        : error instanceof ApiError
          ? error.message
          : strings.errors.unexpected}
    </EmptyState>
  );
}
