import { useMutation, useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { Check, Circle, CircleSlash, RefreshCw, RotateCw, Square, TriangleAlert } from "lucide-react";
import { Suspense, useState } from "react";
import { useNavigate } from "react-router";
import { useCan } from "~/hooks/use-can";
import { useConsoleBuffer } from "~/hooks/use-console-buffer";
import { useJobStream } from "~/hooks/use-job-stream";
import { cancelJob, retryFailedJobs } from "~/lib/api/gen/client";
import type { Job, JobDetail, JobStatus, JobStep, StepStatus } from "~/lib/api/gen/model";
import { dateTime, duration, timeAgo } from "~/lib/format";
import { errorMessage } from "~/lib/forms";
import { jobQuery } from "~/lib/queries";
import { strings } from "~/lib/strings";
import { cn } from "~/lib/utils";
import { Button } from "../ui/button";
import { notify } from "../ui/toast";
import { Breadcrumbs, type Crumb } from "./breadcrumbs";
import { Card, CardHeader } from "./card";
import { Console, ConsoleControls } from "./console";
import { Skeleton } from "./states";
import { type StatusKind, StatusPill } from "./status-pill";

export const jobStatusKind: Record<JobStatus, StatusKind> = {
  queued: "stopped",
  running: "deploying",
  succeeded: "running",
  failed: "failed",
  canceled: "stopped",
};

/**
 * A job's steps and streamed output (JOB-3), with cancel for operators (JOB-4). A project
 * action lists its apps instead of steps, shows the chosen app's output, and offers to
 * retry the apps that failed (JOB-5).
 */
export function JobView({ id }: { id: string }) {
  const { data: job } = useSuspenseQuery(jobQuery(id));
  const canCancel = useCan("job.cancel");
  const canRetry = useCan("project.operate");
  const navigate = useNavigate();
  const group = job.children.length > 0;
  const finished = !["queued", "running"].includes(job.status);
  const t = strings.job;

  const retry = useMutation({
    mutationFn: () => retryFailedJobs(job.id),
    onSuccess: async ({ job_id }) => {
      notify(t.retrying);
      await navigate(`/jobs/${job_id}`);
    },
    onError: (err) => notify(errorMessage(err), "error"),
  });
  const failedChildren = job.children.filter((c) => c.status === "failed" || c.status === "canceled").length;
  const succeededChildren = job.children.filter((c) => c.status === "succeeded").length;

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
            {group ? `${t.summary(succeededChildren, failedChildren, job.children.length)} ` : null}
            {job.started_at ? (
              <time dateTime={job.started_at} title={dateTime(job.started_at)}>
                {t.started(timeAgo(job.started_at), job.creator ?? t.system)}
              </time>
            ) : (
              t.queued(timeAgo(job.created_at))
            )}
          </p>
        </div>
        <div className="flex flex-wrap gap-3">
          {group && canRetry && finished && failedChildren > 0 ? (
            <Button onClick={() => retry.mutate()} disabled={retry.isPending}>
              <RotateCw aria-hidden />
              {t.retryFailed}
            </Button>
          ) : null}
          {canCancel && !finished ? (
            <Button variant="danger" onClick={() => cancel.mutate()} disabled={cancel.isPending}>
              <Square aria-hidden />
              {cancel.isPending ? t.canceling : t.cancel}
            </Button>
          ) : null}
        </div>
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

      {group ? <GroupBody job={job} /> : <SingleBody job={job} />}
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

function SingleBody({ job }: { job: JobDetail }) {
  const t = strings.job;
  return (
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
        <JobOutput job={job} />
      </div>
    </div>
  );
}

/** The output of one job, streamed. */
function JobOutput({ job, title }: { job: JobDetail; title?: string }) {
  const buffer = useConsoleBuffer();
  const { finished } = useJobStream(job, buffer.push);
  const t = strings.job;
  return (
    <Card>
      <CardHeader title={title ?? t.output} />
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
  );
}

/** A project action: its apps on the left, the chosen app's output on the right (JOB-5). */
function GroupBody({ job: initial }: { job: JobDetail }) {
  const t = strings.job;
  // The parent has no output of its own worth streaming; poll it while its apps run.
  const { data: job = initial } = useQuery({
    ...jobQuery(initial.id),
    refetchInterval: (q) =>
      q.state.data && ["queued", "running"].includes(q.state.data.status) ? 2000 : false,
  });
  const firstFailed = job.children.find((c) => c.status === "failed")?.id;
  const [chosen, setChosen] = useState<string | undefined>(undefined);
  const selected =
    chosen ?? firstFailed ?? job.children.find((c) => c.status === "running")?.id ?? job.children[0]?.id;
  return (
    <div className="flex flex-wrap items-start gap-6">
      <div className="w-full max-w-full md:w-[300px]">
        <Card>
          <CardHeader title={t.apps} />
          <ul>
            {job.children.map((c) => (
              <li key={c.id} className="border-t border-line first:border-t-0">
                <button
                  type="button"
                  onClick={() => setChosen(c.id)}
                  aria-pressed={c.id === selected}
                  className={cn(
                    "flex min-h-14 w-full cursor-pointer items-center gap-3 px-5 py-2 text-left",
                    c.id === selected ? "bg-nav-active" : "hover:bg-subtle",
                  )}
                >
                  <ChildIcon status={c.status} />
                  <span className="flex flex-col">
                    <span className="font-semibold">{c.app ?? c.title}</span>
                    <span className="text-[13px] text-muted">{childDetail(c)}</span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </Card>
      </div>
      <div className="min-w-0 flex-[1_1_520px]">
        {selected ? (
          <Suspense fallback={<Skeleton className="h-80" />}>
            <ChildOutput key={selected} id={selected} />
          </Suspense>
        ) : null}
      </div>
    </div>
  );
}

function ChildOutput({ id }: { id: string }) {
  const { data: child } = useSuspenseQuery(jobQuery(id));
  return <JobOutput job={child} title={`${strings.job.output}: ${child.app ?? child.title}`} />;
}

function childDetail(c: Job): string {
  const t = strings.job;
  if (c.status === "succeeded" && c.started_at) return t.stepDone(duration(c.started_at, c.finished_at));
  if (c.status === "failed" && c.started_at) return t.stepFailed(duration(c.started_at, c.finished_at));
  if (c.status === "running" && c.started_at) return t.stepRunning(duration(c.started_at));
  return t.status[c.status];
}

function ChildIcon({ status }: { status: JobStatus }) {
  const s: StepStatus = status;
  const { icon: Icon, className } = stepIcons[s];
  return <Icon className={`size-[18px] shrink-0 ${className}`} aria-hidden />;
}
