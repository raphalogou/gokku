import { useMutation, useSuspenseQuery } from "@tanstack/react-query";
import { Check, Circle, CircleSlash, FileX, RefreshCw, Square, TriangleAlert } from "lucide-react";
import { isRouteErrorResponse, Link, useRouteError } from "react-router";
import { Breadcrumbs, type Crumb } from "~/components/app/breadcrumbs";
import { Card, CardHeader } from "~/components/app/card";
import { Console, ConsoleControls } from "~/components/app/console";
import { EmptyState } from "~/components/app/states";
import { type StatusKind, StatusPill } from "~/components/app/status-pill";
import { Button, buttonVariants } from "~/components/ui/button";
import { notify } from "~/components/ui/toast";
import { useCan } from "~/hooks/use-can";
import { useConsoleBuffer } from "~/hooks/use-console-buffer";
import { useJobStream } from "~/hooks/use-job-stream";
import { ApiError } from "~/lib/api/fetcher";
import { cancelJob } from "~/lib/api/gen/client";
import type { JobStatus, JobStep, StepStatus } from "~/lib/api/gen/model";
import { dateTime, duration, timeAgo } from "~/lib/format";
import { errorMessage } from "~/lib/forms";
import { jobQuery } from "~/lib/queries";
import { queryClient } from "~/lib/query-client";
import { strings } from "~/lib/strings";
import type { Route } from "./+types/job";

export async function clientLoader({ params }: Route.ClientLoaderArgs) {
  await queryClient.ensureQueryData(jobQuery(params.id));
  return null;
}

export const jobStatusKind: Record<JobStatus, StatusKind> = {
  queued: "stopped",
  running: "deploying",
  succeeded: "running",
  failed: "failed",
  canceled: "stopped",
};

/** A job's steps and streamed output (JOB-3), with cancel for operators (JOB-4). */
export default function Job({ params }: Route.ComponentProps) {
  const { data: job } = useSuspenseQuery(jobQuery(params.id));
  const buffer = useConsoleBuffer();
  const { finished } = useJobStream(job, buffer.push);
  const canCancel = useCan("job.cancel");
  const t = strings.job;

  const cancel = useMutation({
    mutationFn: () => cancelJob(job.id),
    onSuccess: () => notify(t.canceled),
    onError: (err) => notify(errorMessage(err), "error"),
  });

  const crumbs: Crumb[] = [];
  if (job.project) {
    crumbs.push({ to: "/projects", label: strings.project.breadcrumb });
    crumbs.push({ to: `/projects/${job.project.slug}`, label: job.project.name });
  } else if (job.app) {
    crumbs.push({ to: "/apps", label: strings.apps.title });
  }
  if (job.app) crumbs.push({ to: `/apps/${job.app}`, label: job.app });
  crumbs.push({ label: job.title });

  return (
    <>
      <Breadcrumbs items={crumbs} />
      <header className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="text-[28px] leading-tight font-bold">{job.title}</h1>
            <StatusPill kind={jobStatusKind[job.status]} label={t.status[job.status]} />
          </div>
          <p className="mt-1 text-[15px] text-muted">
            {job.started_at ? (
              <time dateTime={job.started_at} title={dateTime(job.started_at)}>
                {t.started(timeAgo(job.started_at), job.creator ?? t.system)}
              </time>
            ) : (
              t.queued(timeAgo(job.created_at))
            )}
          </p>
        </div>
        {canCancel && !finished ? (
          <Button variant="danger" onClick={() => cancel.mutate()} disabled={cancel.isPending}>
            <Square aria-hidden />
            {cancel.isPending ? t.canceling : t.cancel}
          </Button>
        ) : null}
      </header>

      {job.error && job.status === "failed" ? (
        <p
          role="alert"
          className="mb-6 rounded-lg border border-failed bg-failed-bg px-4 py-3 text-failed-fg"
        >
          <span className="font-semibold">{t.error}: </span>
          {job.error}
        </p>
      ) : null}

      <div className="flex flex-wrap items-start gap-6">
        <div className="w-full max-w-full md:w-[300px]">
          <Card>
            <CardHeader title={t.steps} />
            <ol>
              {job.steps.map((s) => (
                <StepRow key={s.position} step={s} />
              ))}
            </ol>
          </Card>
        </div>
        <div className="min-w-0 flex-[1_1_520px]">
          <Card>
            <CardHeader title={t.output} />
            <div className="flex flex-col gap-3 p-4">
              <Console lines={buffer.lines} label={t.output} follow={!buffer.paused} empty={t.noOutput} />
              <ConsoleControls
                lines={buffer.lines}
                paused={buffer.paused}
                waiting={buffer.waiting}
                onPausedChange={buffer.setPaused}
                live={!finished}
                filename={`${job.app ?? "job"}-${job.id}.log`}
              />
            </div>
          </Card>
        </div>
      </div>
    </>
  );
}

const stepIcons: Record<StepStatus, { icon: typeof Check; className: string }> = {
  succeeded: { icon: Check, className: "text-running-fg" },
  running: { icon: RefreshCw, className: "text-deploying-fg animate-spin [animation-duration:2s]" },
  queued: { icon: Circle, className: "text-muted" },
  failed: { icon: TriangleAlert, className: "text-failed-fg" },
  skipped: { icon: CircleSlash, className: "text-muted" },
  canceled: { icon: CircleSlash, className: "text-muted" },
};

function StepRow({ step: s }: { step: JobStep }) {
  const t = strings.job;
  const { icon: Icon, className } = stepIcons[s.status];
  let detail: string = t.stepWaiting;
  if (s.status === "succeeded" && s.started_at) detail = t.stepDone(duration(s.started_at, s.finished_at));
  else if (s.status === "running" && s.started_at) detail = t.stepRunning(duration(s.started_at));
  else if (s.status === "failed" && s.started_at)
    detail = t.stepFailed(duration(s.started_at, s.finished_at));
  else if (s.status === "skipped") detail = t.stepSkipped;
  else if (s.status === "canceled") detail = t.stepCanceled;
  return (
    <li className="flex min-h-[52px] items-center gap-3 border-t border-line px-5 py-2 first:border-t-0">
      <Icon className={`size-[18px] shrink-0 ${className}`} aria-hidden />
      <div className="flex flex-col">
        <span className="font-semibold">{s.name}</span>
        <span className="text-[13px] text-muted">
          <span className="sr-only">{t.status[s.status]}: </span>
          {detail}
        </span>
      </div>
    </li>
  );
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
