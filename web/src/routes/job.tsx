import { FileX } from "lucide-react";
import { isRouteErrorResponse, Link, useRouteError } from "react-router";
import { JobView } from "~/components/app/job-view";
import { EmptyState } from "~/components/app/states";
import { buttonVariants } from "~/components/ui/button";
import { ApiError } from "~/lib/api/fetcher";
import { jobQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/job";

export async function clientLoader({ params }: Route.ClientLoaderArgs) {
  await queryClient.ensureQueryData(jobQuery(params.id));
  return null;
}

/** /jobs/:id: any job, including a project action's progress (DESIGN 7). */
export default function Job({ params }: Route.ComponentProps) {
  return <JobView id={params.id} />;
}

export function ErrorBoundary() {
  const error = useRouteError();
  const notFound =
    (error instanceof ApiError && error.status === 404) ||
    (isRouteErrorResponse(error) && error.status === 404);
  return (
    <EmptyState
      icon={FileX}
      title={notFound ? strings.errors.notFoundTitle : strings.errors.loadFailed}
      action={
        <Link to="/activity" className={buttonVariants({ variant: "secondary" })}>
          {strings.nav.activity}
        </Link>
      }
    >
      {error instanceof ApiError ? error.message : strings.errors.unexpected}
    </EmptyState>
  );
}
