import { JobView } from "~/components/app/job-view";
import { jobQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import type { Route } from "./+types/deploy-output";

export { ErrorBoundary } from "./job";

export async function clientLoader({ params }: Route.ClientLoaderArgs) {
  await queryClient.ensureQueryData(jobQuery(params.jobId));
  return null;
}

/** /apps/:name/deploys/:jobId: a deploy's steps and output (DEP-2). */
export default function DeployOutput({ params }: Route.ComponentProps) {
  return <JobView id={params.jobId} />;
}
